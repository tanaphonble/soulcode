package openai_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tanaphonble/soulcode/internal/provider"
	"github.com/tanaphonble/soulcode/internal/provider/openai"
)

func newTestClient(t *testing.T, baseURL string) *openai.Client {
	t.Helper()
	c, err := openai.New("test-key", "test-model", baseURL)
	if err != nil {
		t.Fatalf("openai.New: %v", err)
	}
	return c
}

func sseServer(events ...string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, e := range events {
			fmt.Fprintf(w, "data: %s\n\n", e) //nolint:errcheck // test server write
		}
		fmt.Fprintf(w, "data: [DONE]\n\n") //nolint:errcheck
	}))
}

func TestChat_TextResponse(t *testing.T) {
	t.Parallel()
	srv := sseServer(
		`{"choices":[{"delta":{"role":"assistant","content":"Hello"},"finish_reason":null}]}`,
		`{"choices":[{"delta":{"content":" world"},"finish_reason":null}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	)
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	ch, err := c.Chat(context.Background(), []provider.Message{
		{Role: provider.RoleUser, Content: "hi"},
	}, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	var got strings.Builder
	for ev := range ch {
		if ev.Err != nil {
			t.Fatalf("unexpected error: %v", ev.Err)
		}
		got.WriteString(ev.Text)
	}
	if got.String() != "Hello world" {
		t.Errorf("expected 'Hello world', got %q", got.String())
	}
}

func TestChat_ToolCallResponse(t *testing.T) {
	t.Parallel()
	srv := sseServer(
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_01","type":"function","function":{"name":"bash","arguments":""}}]},"finish_reason":null}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"command\":"}}]},"finish_reason":null}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"ls\"}"}}]},"finish_reason":null}]}`,
		`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
	)
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	ch, err := c.Chat(context.Background(), []provider.Message{
		{Role: provider.RoleUser, Content: "list files"},
	}, []provider.Tool{{Name: "bash", Description: "run shell", Schema: []byte(`{}`)}})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	var toolCall *provider.ToolCall
	for ev := range ch {
		if ev.Err != nil {
			t.Fatalf("unexpected error: %v", ev.Err)
		}
		if ev.ToolCall != nil {
			toolCall = ev.ToolCall
		}
	}

	if toolCall == nil {
		t.Fatal("expected a tool call event, got none")
	}
	if toolCall.Name != "bash" {
		t.Errorf("expected tool 'bash', got %q", toolCall.Name)
	}
	if !strings.Contains(string(toolCall.Input), "ls") {
		t.Errorf("expected 'ls' in input, got %q", toolCall.Input)
	}
}

func TestChat_APIError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, `{"error":{"message":"invalid api key"}}`) //nolint:errcheck
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	ch, err := c.Chat(context.Background(), []provider.Message{
		{Role: provider.RoleUser, Content: "hi"},
	}, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	var gotErr error
	for ev := range ch {
		if ev.Err != nil {
			gotErr = ev.Err
		}
	}
	if gotErr == nil {
		t.Error("expected error event for 401 response")
	}
	if !strings.Contains(gotErr.Error(), "invalid api key") {
		t.Errorf("unexpected error: %v", gotErr)
	}
}

func TestChat_ContextCancellation(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := c.Chat(ctx, []provider.Message{{Role: provider.RoleUser, Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	cancel()
	for range ch {
	}
}

func TestNew_RequiresModel(t *testing.T) {
	t.Parallel()
	_, err := openai.New("key", "", "")
	if err == nil {
		t.Error("expected error when model is empty")
	}
}
