package anthropic_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tanaphonble/soulcode/internal/provider"
	"github.com/tanaphonble/soulcode/internal/provider/anthropic"
)

// newTestClient creates a client pointing at a test server URL.
func newTestClient(t *testing.T, url string) *anthropic.Client {
	t.Helper()
	c, err := anthropic.New("test-key", "test-model")
	if err != nil {
		t.Fatalf("anthropic.New: %v", err)
	}
	c.SetAPIURL(url)
	return c
}

func sseServer(events ...string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, e := range events {
			fmt.Fprintf(w, "data: %s\n\n", e) //nolint:errcheck // test server write, can't fail
		}
	}))
}

func TestChat_TextResponse(t *testing.T) {
	t.Parallel()
	srv := sseServer(
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_stop"}`,
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
			t.Fatalf("unexpected error event: %v", ev.Err)
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
		`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_01","name":"bash","input":{}}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"command\":"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\"ls\"}"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_stop"}`,
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
		t.Errorf("expected tool name 'bash', got %q", toolCall.Name)
	}
	if !strings.Contains(string(toolCall.Input), "ls") {
		t.Errorf("expected 'ls' in tool input, got %q", toolCall.Input)
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
		t.Errorf("unexpected error message: %v", gotErr)
	}
}

func TestChat_ContextCancellation(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// block until client disconnects
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
	} // must drain cleanly without blocking forever
}

func TestNew_RequiresAPIKey(t *testing.T) {
	t.Parallel()
	_, err := anthropic.New("", "claude-sonnet-4-6")
	if err == nil {
		t.Error("expected error when API key is empty")
	}
}
