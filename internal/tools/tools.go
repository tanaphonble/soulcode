// Package tools implements the built-in tool registry for soulcode.
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"soulcode/internal/provider"
	"soulcode/internal/security"
)

// executeFn is the signature for a tool's implementation. It receives both the
// JSON input from the LLM and the runtime security context that scopes the
// tool's access to the filesystem and shell.
type executeFn func(ctx context.Context, input json.RawMessage, sec *SecurityContext) (string, error)

// entry holds a tool definition alongside its implementation.
type entry struct {
	def provider.Tool
	run executeFn
}

// SecurityContext carries the workdir boundary, allow-list, approver, and
// auditor that all tool invocations must respect. A nil approver disables
// confirmation prompts (acceptable in tests; main.go always supplies one).
type SecurityContext struct {
	Workdir   string
	Approver  security.Approver
	AllowList *security.AllowList
	Auditor   *security.Auditor
}

// Registry holds all available tools and dispatches LLM tool calls.
type Registry struct {
	tools map[string]*entry
	sec   *SecurityContext
}

// New returns a Registry pre-loaded with all built-in tools and bound to the
// supplied security context.
func New(sec *SecurityContext) *Registry {
	if sec == nil {
		sec = &SecurityContext{}
	}
	r := &Registry{tools: map[string]*entry{}, sec: sec}
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
	return e.run(ctx, call.Input, r.sec)
}

// schema builds a JSON Schema literal.
func schema(s string) json.RawMessage { return json.RawMessage(s) }
