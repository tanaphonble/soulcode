// Package tools implements the built-in tool registry for soulcode.
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"soulcode/internal/provider"
)

// executeFn is the signature for a tool's implementation.
type executeFn func(ctx context.Context, input json.RawMessage) (string, error)

// entry holds a tool definition alongside its implementation.
type entry struct {
	def provider.Tool
	run executeFn
}

// Registry holds all available tools and dispatches LLM tool calls.
type Registry struct {
	tools map[string]*entry
}

// New returns a Registry pre-loaded with all built-in tools.
func New() *Registry {
	r := &Registry{tools: map[string]*entry{}}
	r.register(thinkTool())
	r.register(bashTool())
	r.register(readFileTool())
	r.register(writeFileTool())
	r.register(editFileTool())
	r.register(lsTool())
	r.register(grepTool())
	r.register(globTool())
	return r
}

func (r *Registry) register(def provider.Tool, run executeFn) {
	r.tools[def.Name] = &entry{def: def, run: run}
}

// Definitions returns the list of tool definitions to pass to the LLM.
func (r *Registry) Definitions() []provider.Tool {
	out := make([]provider.Tool, 0, len(r.tools))
	for _, e := range r.tools {
		out = append(out, e.def)
	}
	return out
}

// Execute runs the tool named by call.Name with call.Input.
func (r *Registry) Execute(ctx context.Context, call provider.ToolCall) (string, error) {
	e, ok := r.tools[call.Name]
	if !ok {
		return "", fmt.Errorf("unknown tool %q", call.Name)
	}
	return e.run(ctx, call.Input)
}

// schema builds a JSON Schema literal.
func schema(s string) json.RawMessage { return json.RawMessage(s) }
