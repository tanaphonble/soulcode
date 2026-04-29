// Package openai implements the provider interface for any OpenAI-compatible API:
// OpenAI, Groq, Together, Mistral, Ollama, and others.
package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"soulcode/internal/provider"
)

var httpClient = &http.Client{
	Timeout: 10 * time.Minute,
	Transport: &http.Transport{
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	},
}

const defaultBaseURL = "https://api.openai.com/v1"

// Client is an OpenAI-compatible streaming client.
type Client struct {
	apiKey  string
	model   string
	baseURL string
	http    *http.Client
}

// New returns a Client. baseURL defaults to the official OpenAI endpoint if empty.
// apiKey may be empty for local endpoints that need no authentication.
func New(apiKey, model, baseURL string) (*Client, error) {
	if model == "" {
		return nil, fmt.Errorf("openai: model name required")
	}
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	base := strings.TrimRight(baseURL, "/")
	if err := validateBaseURL(base); err != nil {
		return nil, err
	}
	return &Client{
		apiKey:  apiKey,
		model:   model,
		baseURL: base,
		http:    httpClient,
	}, nil
}

// validateBaseURL ensures the base URL uses HTTPS, or HTTP only for loopback addresses.
// Uses proper URL parsing to prevent hostname bypass attacks (e.g. http://localhost.evil.com).
func validateBaseURL(base string) error {
	u, err := url.Parse(base)
	if err != nil {
		return fmt.Errorf("openai: invalid base URL: %w", err)
	}
	if u.Scheme == "https" {
		return nil
	}
	if u.Scheme == "http" {
		host := u.Hostname() // strips port
		if host == "localhost" || host == "127.0.0.1" || host == "::1" {
			return nil
		}
		return fmt.Errorf("openai: HTTP only allowed for localhost, got host %q", host)
	}
	return fmt.Errorf("openai: base URL must use HTTPS (or http://localhost for local endpoints), got %q", base)
}

func (c *Client) ID() string { return c.baseURL + "/" + c.model }

func (c *Client) Chat(ctx context.Context, messages []provider.Message, tools []provider.Tool) (<-chan provider.Event, error) {
	body, err := c.buildRequest(messages, tools)
	if err != nil {
		return nil, fmt.Errorf("openai: build request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	ch := make(chan provider.Event, 8)
	go c.stream(ctx, req, ch)
	return ch, nil
}

// ── Request serialisation ────────────────────────────────────────────────────

type oaiToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type oaiMsg struct {
	Role       string        `json:"role"`
	Content    *string       `json:"content"` // pointer so empty string vs null are distinct
	ToolCalls  []oaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
}

func strPtr(s string) *string { return &s }

func (c *Client) buildRequest(messages []provider.Message, tools []provider.Tool) ([]byte, error) {

	msgs := make([]oaiMsg, 0, len(messages))
	for _, m := range messages {
		switch m.Role {
		case provider.RoleTool:
			msgs = append(msgs, oaiMsg{
				Role:       "tool",
				Content:    strPtr(m.Content),
				ToolCallID: m.ToolCallID,
			})
		case provider.RoleAssistant:
			msg := oaiMsg{Role: "assistant", Content: strPtr(m.Content)}
			for _, tc := range m.ToolCalls {
				args := string(tc.Input)
				if args == "" {
					args = "{}"
				}
				tc2 := oaiToolCall{ID: tc.ID, Type: "function"}
				tc2.Function.Name = tc.Name
				tc2.Function.Arguments = args
				msg.ToolCalls = append(msg.ToolCalls, tc2)
			}
			msgs = append(msgs, msg)
		default:
			msgs = append(msgs, oaiMsg{Role: string(m.Role), Content: strPtr(m.Content)})
		}
	}

	payload := map[string]any{
		"model":    c.model,
		"stream":   true,
		"messages": msgs,
	}
	if len(tools) > 0 {
		payload["tools"] = openAITools(tools)
	}

	return json.Marshal(payload)
}

func openAITools(tools []provider.Tool) []map[string]any {
	out := make([]map[string]any, len(tools))
	for i, t := range tools {
		out[i] = map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  t.Schema,
			},
		}
	}
	return out
}

// ── Streaming parser ─────────────────────────────────────────────────────────

// partialToolCall accumulates a streaming tool call before it's complete.
type partialToolCall struct {
	id   string
	name string
	args strings.Builder
}

// streamChunk is one OpenAI-compatible SSE row.
type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

func (c *Client) stream(ctx context.Context, req *http.Request, ch chan<- provider.Event) {
	defer close(ch)

	resp, err := c.http.Do(req) //nolint:gosec // URL validated in New()
	if err != nil {
		if ctx.Err() == nil {
			ch <- provider.Event{Err: fmt.Errorf("openai: %w", err)}
		}
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		ch <- provider.Event{Err: decodeStreamError(resp)}
		return
	}

	pending := map[int]*partialToolCall{}
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		if ctx.Err() != nil {
			return
		}
		data, ok := strings.CutPrefix(scanner.Text(), "data: ")
		if !ok || data == "" {
			continue
		}
		if data == "[DONE]" {
			flushToolCalls(pending, ch)
			return
		}
		var c streamChunk
		if err := json.Unmarshal([]byte(data), &c); err != nil || len(c.Choices) == 0 {
			continue
		}
		if done := applyChoice(c.Choices[0], pending, ch); done {
			flushToolCalls(pending, ch)
			return
		}
	}
	flushToolCalls(pending, ch)
}

// decodeStreamError pulls the human-readable message out of an error response.
func decodeStreamError(resp *http.Response) error {
	var body struct {
		Error struct{ Message string } `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	msg := body.Error.Message
	if msg == "" {
		msg = resp.Status
	}
	return fmt.Errorf("openai: %s", msg)
}

// applyChoice processes one delta and returns true when finish_reason indicates
// the stream is complete.
func applyChoice(choice struct {
	Delta struct {
		Content   string `json:"content"`
		ToolCalls []struct {
			Index    int    `json:"index"`
			ID       string `json:"id"`
			Type     string `json:"type"`
			Function struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls"`
	} `json:"delta"`
	FinishReason *string `json:"finish_reason"`
}, pending map[int]*partialToolCall, ch chan<- provider.Event) bool {
	if choice.FinishReason != nil {
		return true
	}
	if text := choice.Delta.Content; text != "" {
		ch <- provider.Event{Text: text}
	}
	for _, tc := range choice.Delta.ToolCalls {
		p, exists := pending[tc.Index]
		if !exists {
			p = &partialToolCall{}
			pending[tc.Index] = p
		}
		if tc.ID != "" {
			p.id = tc.ID
		}
		if tc.Function.Name != "" {
			p.name = tc.Function.Name
		}
		p.args.WriteString(tc.Function.Arguments)
	}
	return false
}

func flushToolCalls(pending map[int]*partialToolCall, ch chan<- provider.Event) {
	for i := range len(pending) {
		p, ok := pending[i]
		if !ok {
			continue
		}
		args := json.RawMessage(p.args.String())
		if len(args) == 0 {
			args = json.RawMessage("{}")
		}
		ch <- provider.Event{ToolCall: &provider.ToolCall{
			ID:    p.id,
			Name:  p.name,
			Input: args,
		}}
	}
}
