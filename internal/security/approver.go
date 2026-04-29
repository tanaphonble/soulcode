package security

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/chzyer/readline"
)

// Decision is the user's response to an approval prompt.
type Decision int

const (
	// DecisionDeny rejects the request. The tool must NOT run.
	DecisionDeny Decision = iota
	// DecisionAllowOnce allows this single invocation.
	DecisionAllowOnce
	// DecisionAllowAlways allows this invocation and silently allows all
	// future invocations of the same tool for the rest of the session.
	// Dangerous-pattern matches still re-prompt regardless.
	DecisionAllowAlways
)

// Approver gates tool execution behind a human confirmation. The default
// implementation (NewReadlineApprover) prompts on the same readline instance
// used by the REPL.
type Approver interface {
	// Approve asks the user whether tool may run with the given action
	// description. dangerous=true forces a prompt even when the session is
	// in "always allow" mode.
	Approve(ctx context.Context, tool, action string, dangerous bool) Decision
}

// ReadlineApprover prompts via the existing readline channel. It expects the
// REPL goroutine to have already called rl.Readline() once and to be waiting
// on lineCh (this is the normal REPL state when an agent loop is running).
type ReadlineApprover struct {
	rl            *readline.Instance
	lineCh        <-chan string
	yolo          bool
	defaultPrompt string

	mu     sync.Mutex
	always map[string]bool // per-tool, session-scoped
}

// NewReadlineApprover wires an Approver around a readline instance and the
// REPL's input channel. yolo=true skips ALL prompts except dangerous-pattern
// matches.
func NewReadlineApprover(rl *readline.Instance, lineCh <-chan string, defaultPrompt string, yolo bool) *ReadlineApprover {
	return &ReadlineApprover{
		rl:            rl,
		lineCh:        lineCh,
		yolo:          yolo,
		defaultPrompt: defaultPrompt,
		always:        map[string]bool{},
	}
}

// Approve implements Approver.
func (a *ReadlineApprover) Approve(ctx context.Context, tool, action string, dangerous bool) Decision {
	if dangerous {
		return a.prompt(ctx, tool, action, true)
	}
	if a.yolo {
		return DecisionAllowOnce
	}
	a.mu.Lock()
	always := a.always[tool]
	a.mu.Unlock()
	if always {
		return DecisionAllowOnce
	}
	return a.prompt(ctx, tool, action, false)
}

func (a *ReadlineApprover) prompt(ctx context.Context, tool, action string, dangerous bool) Decision {
	const reset = "\033[0m"
	const yellow = "\033[33m"
	const red = "\033[31m"
	const bold = "\033[1m"

	// One indented prompt line that flows directly under the tool header
	// already printed by the UI renderer. No separate banner needed.
	options := "[y/N/always]"
	if dangerous {
		fmt.Printf("  %s%s⚠ DANGEROUS%s %s%s%s ",
			red, bold, reset,
			red, "[y/N]", reset)
	} else {
		fmt.Printf("  %sApprove?%s %s%s%s ", yellow, reset, yellow, options, reset)
	}
	_ = action // shown in the renderer's tool header above this prompt

	if a.rl != nil {
		a.rl.SetPrompt("")
		a.rl.Refresh()
		defer func() {
			a.rl.SetPrompt(a.defaultPrompt)
			a.rl.Refresh()
		}()
	}

	select {
	case line, ok := <-a.lineCh:
		if !ok {
			return DecisionDeny
		}
		return parseAnswer(line, tool, dangerous, a)
	case <-ctx.Done():
		return DecisionDeny
	}
}

func parseAnswer(line, tool string, dangerous bool, a *ReadlineApprover) Decision {
	answer := strings.ToLower(strings.TrimSpace(line))
	switch {
	case answer == "y" || answer == "yes":
		return DecisionAllowOnce
	case (answer == "a" || answer == "always") && !dangerous:
		a.mu.Lock()
		a.always[tool] = true
		a.mu.Unlock()
		return DecisionAllowAlways
	default:
		return DecisionDeny
	}
}

// AutoApprover always allows. Used in tests and explicitly opt-in scripts.
type AutoApprover struct{}

// Approve always returns DecisionAllowOnce except for dangerous commands,
// which still get denied (so tests cannot accidentally let a dangerous
// pattern through).
func (AutoApprover) Approve(_ context.Context, _, _ string, dangerous bool) Decision {
	if dangerous {
		return DecisionDeny
	}
	return DecisionAllowOnce
}

// EnvYolo reports whether SOULCODE_YOLO=1 is set in the environment.
func EnvYolo() bool {
	v := os.Getenv("SOULCODE_YOLO")
	return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
}
