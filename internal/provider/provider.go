// Package provider defines the interface for LLM backends.
package provider

import (
	"context"
	"encoding/json"
)

// Role identifies the speaker of a conversation turn.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool" // carries a tool result
)

// ToolCall represents a single tool invocation requested by the LLM.
type ToolCall struct {
	ID    string
	Name  string
	Input json.RawMessage
}

// Message is one turn in a conversation. Depending on Role:
//   - RoleUser/RoleSystem/RoleAssistant: Content holds text.
//   - RoleAssistant with ToolCalls: LLM requested tool executions.
//   - RoleTool: Content is the tool output; ToolCallID links to the request.
type Message struct {
	Role       Role
	Content    string
	ToolCalls  []ToolCall
	ToolCallID string // set when Role == RoleTool
}

// Tool is the provider-agnostic definition of a capability the LLM can invoke.
// Schema must be a valid JSON Schema object describing the input parameters.
type Tool struct {
	Name        string
	Description string
	Schema      json.RawMessage
}

// Event is a single item emitted during streaming.
// Text events may arrive many times; ToolCall events arrive once per call,
// fully assembled. Channel close signals a clean end of stream.
type Event struct {
	Text     string
	ToolCall *ToolCall
	Err      error
}

// Provider is the interface implemented by all LLM backends.
type Provider interface {
	// ID returns a human-readable identifier, e.g. "anthropic/claude-sonnet-4-6".
	ID() string
	// Chat sends the conversation and streams back events.
	// tools may be nil if the caller does not need tool use.
	Chat(ctx context.Context, messages []Message, tools []Tool) (<-chan Event, error)
}
