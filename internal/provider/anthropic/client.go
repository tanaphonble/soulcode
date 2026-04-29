// Package anthropic implements the provider interface for Anthropic's Claude API.
package anthropic

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"soulcode/internal/provider"
)

// httpClient is a shared client with conservative timeouts.
// The read timeout is intentionally long to support streaming responses.
var httpClient = &http.Client{
	Timeout: 10 * time.Minute, // covers long streaming responses
	Transport: &http.Transport{
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	},
}

const (
	defaultModel  = "claude-sonnet-4-6"
	defaultAPIURL = "https://api.anthropic.com/v1/messages"
	apiVersion    = "2023-06-01"
	defaultMaxTok = 8096
)

// Client is an Anthropic API client.
type Client struct {
	apiKey string
	model  string
	apiURL string
	http   *http.Client
}

// New returns a Client. model defaults to claude-sonnet-4-6 if empty.
func New(apiKey, model string) (*Client, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("anthropic: API key required")
	}
	if model == "" {
		model = defaultModel
	}
	return &Client{apiKey: apiKey, model: model, apiURL: defaultAPIURL, http: httpClient}, nil
}

func (c *Client) ID() string { return "anthropic/" + c.model }

// SetAPIURL overrides the default API endpoint. Intended for testing.
func (c *Client) SetAPIURL(url string) { c.apiURL = url }

func (c *Client) Chat(ctx context.Context, messages []provider.Message, tools []provider.Tool) (<-chan provider.Event, error) {
	body, err := c.buildRequest(messages, tools)
	if err != nil {
		return nil, fmt.Errorf("anthropic: build request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("anthropic: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", apiVersion)

	ch := make(chan provider.Event, 8)
	go c.stream(ctx, req, ch)
	return ch, nil
}

// ── Request serialisation ────────────────────────────────────────────────────

func (c *Client) buildRequest(messages []provider.Message, tools []provider.Tool) ([]byte, error) {
	type textPart struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	type toolUsePart struct {
		Type  string          `json:"type"`
		ID    string          `json:"id"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	}
	type toolResultPart struct {
		Type      string `json:"type"`
		ToolUseID string `json:"tool_use_id"`
		Content   string `json:"content"`
	}

	type msg struct {
		Role    string `json:"role"`
		Content any    `json:"content"`
	}

	var system string
	var msgs []msg

	for _, m := range messages {
		switch m.Role {
		case provider.RoleSystem:
			system = m.Content

		case provider.RoleUser:
			msgs = append(msgs, msg{Role: "user", Content: m.Content})

		case provider.RoleAssistant:
			if len(m.ToolCalls) == 0 {
				msgs = append(msgs, msg{Role: "assistant", Content: m.Content})
				continue
			}
			// Build a content array: optional text + tool_use blocks.
			var parts []any
			if m.Content != "" {
				parts = append(parts, textPart{Type: "text", Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				input := tc.Input
				if input == nil {
					input = json.RawMessage("{}")
				}
				parts = append(parts, toolUsePart{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Name,
					Input: input,
				})
			}
			msgs = append(msgs, msg{Role: "assistant", Content: parts})

		case provider.RoleTool:
			// Anthropic expects tool results as a user message.
			msgs = append(msgs, msg{
				Role: "user",
				Content: []any{toolResultPart{
					Type:      "tool_result",
					ToolUseID: m.ToolCallID,
					Content:   m.Content,
				}},
			})
		}
	}

	payload := map[string]any{
		"model":      c.model,
		"max_tokens": defaultMaxTok,
		"stream":     true,
		"messages":   msgs,
	}
	if system != "" {
		payload["system"] = system
	}
	if len(tools) > 0 {
		payload["tools"] = anthropicTools(tools)
	}

	return json.Marshal(payload)
}

// anthropicTools converts generic tool definitions to Anthropic's format.
func anthropicTools(tools []provider.Tool) []map[string]any {
	out := make([]map[string]any, len(tools))
	for i, t := range tools {
		out[i] = map[string]any{
			"name":         t.Name,
			"description":  t.Description,
			"input_schema": t.Schema,
		}
	}
	return out
}

// ── Streaming parser ─────────────────────────────────────────────────────────

// block tracks an in-progress content block during streaming.
type block struct {
	kind     string // "text" or "tool_use"
	id       string
	name     string
	inputBuf strings.Builder
}

func (c *Client) stream(ctx context.Context, req *http.Request, ch chan<- provider.Event) {
	defer close(ch)

	resp, err := c.http.Do(req) //nolint:gosec // URL validated in New()
	if err != nil {
		if ctx.Err() == nil {
			ch <- provider.Event{Err: fmt.Errorf("anthropic: %w", err)}
		}
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		var body struct {
			Error struct{ Message string } `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&body)
		msg := body.Error.Message
		if msg == "" {
			msg = resp.Status
		}
		ch <- provider.Event{Err: fmt.Errorf("anthropic: %s", msg)}
		return
	}

	blocks := map[int]*block{}

	type rawEvent struct {
		Type  string `json:"type"`
		Index int    `json:"index"`
		// content_block_start
		ContentBlock struct {
			Type string `json:"type"`
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"content_block"`
		// content_block_delta
		Delta struct {
			Type        string `json:"type"`
			Text        string `json:"text"`
			PartialJSON string `json:"partial_json"`
		} `json:"delta"`
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		if ctx.Err() != nil {
			return
		}
		data, ok := strings.CutPrefix(scanner.Text(), "data: ")
		if !ok || data == "" {
			continue
		}

		var ev rawEvent
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			continue
		}

		switch ev.Type {
		case "content_block_start":
			b := &block{kind: ev.ContentBlock.Type, id: ev.ContentBlock.ID, name: ev.ContentBlock.Name}
			blocks[ev.Index] = b

		case "content_block_delta":
			b := blocks[ev.Index]
			if b == nil {
				continue
			}
			switch ev.Delta.Type {
			case "text_delta":
				if ev.Delta.Text != "" {
					ch <- provider.Event{Text: ev.Delta.Text}
				}
			case "input_json_delta":
				b.inputBuf.WriteString(ev.Delta.PartialJSON)
			}

		case "content_block_stop":
			b := blocks[ev.Index]
			if b == nil || b.kind != "tool_use" {
				continue
			}
			input := json.RawMessage(b.inputBuf.String())
			if len(input) == 0 {
				input = json.RawMessage("{}")
			}
			ch <- provider.Event{ToolCall: &provider.ToolCall{
				ID:    b.id,
				Name:  b.name,
				Input: input,
			}}

		case "message_stop":
			return
		}
	}
}
