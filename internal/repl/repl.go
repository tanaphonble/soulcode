// Package repl implements the interactive read-eval-print loop.
package repl

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/chzyer/readline"

	sctx "soulcode/internal/context"
	"soulcode/internal/provider"
	"soulcode/internal/session"
	"soulcode/internal/tools"
)

// ANSI escape codes — no external dependency needed.
const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiDim    = "\033[2m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiRed    = "\033[31m"
	ansiCyan   = "\033[36m"
	promptStr  = ansiGreen + ansiBold + ">" + ansiReset + " "
)

// SwitchFn creates a new provider for a given model name.
// It is called when the user runs /model <name>.
type SwitchFn func(model string) (provider.Provider, error)

// REPL is an interactive session with an LLM provider.
type REPL struct {
	p        provider.Provider
	switchFn SwitchFn
	sess     *session.Session
	tools    *tools.Registry
	workDir  string
	restored bool
}

const systemPrompt = `You are soulcode, an expert software engineering assistant running in the terminal.

## Before implementing anything

For any non-trivial task (new project, new feature, architectural decision), ask clarifying questions FIRST. Do not write a single line of code until you understand:
- The exact requirements and constraints
- Tech stack preferences (framework, ORM, etc.)
- Database schema if relevant
- Whether tests are required and what kind

Keep questions short and grouped. One round of questions is enough — then execute completely.

## When implementing

- Write complete, production-ready code. Never write placeholders, stubs, or "// TODO" comments.
- Always run bash to verify: compile, test, lint. Fix errors before reporting done.
- After adding any dependency, run go mod tidy (Go), npm install (Node), etc.
- Always read a file before editing it.
- Prefer edit_file over write_file for existing files.
- Write idiomatic code for the language and framework of the project.
- Never truncate code — write every line.

## Output style

- No lengthy explanations or step-by-step narration. Just do the work.
- After finishing, give a short summary of what was done and how to run it.
- If something is ambiguous mid-task, make a reasonable decision and note it briefly.`

// New creates a REPL backed by the given provider.
// switchFn is called when the user runs /model <name> — it receives the new
// model name and returns a fresh provider (keeping the same credentials).
func New(p provider.Provider, switchFn SwitchFn) *REPL {
	proj := sctx.Gather()
	sess := session.New(proj.SystemPrompt(systemPrompt))

	restored, _ := sess.Load(proj.WorkDir)

	r := &REPL{
		p:        p,
		switchFn: switchFn,
		sess:     sess,
		tools:    tools.New(),
		workDir:  proj.WorkDir,
	}
	r.restored = restored
	return r
}

// Run starts the REPL and blocks until the user exits or ctx is cancelled.
func (r *REPL) Run(ctx context.Context) error {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	defer signal.Stop(sigCh)

	rl, err := readline.NewEx(&readline.Config{
		Prompt:          promptStr,
		HistoryFile:     historyPath(),
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
	})
	if err != nil {
		return fmt.Errorf("readline: %w", err)
	}
	defer rl.Close() //nolint:errcheck

	fmt.Printf("%ssoulcode%s  %s%s%s\n", ansiBold, ansiReset, ansiDim, r.p.ID(), ansiReset)
	fmt.Printf("%sCtrl+C%s interrupts · %sCtrl+D%s exits · %s/help%s for commands\n",
		ansiBold, ansiReset, ansiBold, ansiReset, ansiBold, ansiReset)
	if r.restored {
		fmt.Printf("%sSession restored (%d messages) — /clear to start fresh%s\n", ansiDim, r.sess.Len(), ansiReset)
	}
	fmt.Println()

	lineCh := make(chan string)
	go func() {
		for {
			line, err := rl.Readline()
			if err == readline.ErrInterrupt {
				sigCh <- os.Interrupt
				continue
			}
			if err == io.EOF || err != nil {
				close(lineCh)
				return
			}
			lineCh <- line
		}
	}()

	for {
		select {
		case <-ctx.Done():
			fmt.Println()
			return nil

		case <-sigCh:
			fmt.Println()
			fmt.Printf("%s[interrupted — type /continue to resume or ask a new question]%s\n", ansiDim, ansiReset)

		case line, ok := <-lineCh:
			if !ok {
				fmt.Println()
				return nil
			}
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if done := r.dispatch(ctx, line, sigCh); done {
				return nil
			}
		}
	}
}

// dispatch handles a single input line. Returns true if the user requested exit.
func (r *REPL) dispatch(ctx context.Context, line string, sigCh <-chan os.Signal) bool {
	if strings.HasPrefix(line, "/") {
		return r.command(ctx, line, sigCh)
	}
	r.agentLoop(ctx, line, sigCh)
	r.sess.Save(r.workDir) //nolint:errcheck,gosec
	return false
}

// command executes a slash command. Returns true if it's an exit command.
func (r *REPL) command(ctx context.Context, line string, sigCh <-chan os.Signal) bool {
	parts := strings.Fields(line)
	switch parts[0] {
	case "/exit", "/quit":
		return true

	case "/clear":
		r.sess.Clear()
		r.sess.DeleteSaved(r.workDir)
		fmt.Printf("%sConversation cleared.%s\n", ansiDim, ansiReset)

	case "/continue":
		r.agentLoop(ctx, "Continue your previous response from where you left off.", sigCh)

	case "/model":
		r.handleModel(parts[1:])

	case "/help":
		fmt.Print(helpText)

	default:
		fmt.Printf("%sunknown command %q — try /help%s\n", ansiYellow, parts[0], ansiReset)
	}
	return false
}

// handleModel handles the /model command:
//
//	/model           — print current model
//	/model <name>    — switch to a new model (keeps session history)
func (r *REPL) handleModel(args []string) {
	if len(args) == 0 {
		fmt.Printf("current model: %s%s%s\n", ansiBold, r.p.ID(), ansiReset)
		return
	}
	model := args[0]
	if r.switchFn == nil {
		fmt.Printf("%s/model switching not available%s\n", ansiRed, ansiReset)
		return
	}
	p, err := r.switchFn(model)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%serror: %v%s\n", ansiRed, err, ansiReset)
		return
	}
	r.p = p
	fmt.Printf("%sswitched to %s%s%s\n", ansiDim, ansiBold, r.p.ID(), ansiReset)
}

// agentLoop runs the full agentic cycle: LLM → tools → LLM → ... until the
// LLM responds with no tool calls or the user interrupts.
func (r *REPL) agentLoop(ctx context.Context, input string, sigCh <-chan os.Signal) {
	r.sess.Add(provider.Message{Role: provider.RoleUser, Content: input})

	for {
		text, calls, interrupted := r.streamOneTurn(ctx, sigCh)

		r.sess.Add(provider.Message{
			Role:      provider.RoleAssistant,
			Content:   text,
			ToolCalls: calls,
		})

		if interrupted {
			fmt.Printf("%s[interrupted — /continue to resume]%s\n", ansiDim, ansiReset)
			return
		}
		if len(calls) == 0 {
			return
		}

		for _, call := range calls {
			fmt.Printf("\n%s[%s]%s\n", ansiCyan, call.Name, ansiReset)
			result, err := r.tools.Execute(ctx, call)
			if err != nil {
				result = fmt.Sprintf("error: %v", err)
				fmt.Printf("%s%s%s\n", ansiRed, result, ansiReset)
			} else {
				fmt.Printf("%s%s%s\n", ansiDim, truncate(result, 400), ansiReset)
			}
			if result == "" {
				result = "(no output)"
			}
			r.sess.Add(provider.Message{
				Role:       provider.RoleTool,
				Content:    result,
				ToolCallID: call.ID,
			})
		}
		fmt.Println()
	}
}

// streamOneTurn sends the current session to the LLM and streams one response.
// Returns the accumulated text, any tool calls, and whether the user interrupted.
func (r *REPL) streamOneTurn(ctx context.Context, sigCh <-chan os.Signal) (text string, calls []provider.ToolCall, interrupted bool) {
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	ch, err := r.p.Chat(streamCtx, r.sess.Messages(), r.tools.Definitions())
	if err != nil {
		fmt.Fprintf(os.Stderr, "%serror: %v%s\n", ansiRed, err, ansiReset)
		return "", nil, false
	}

	fmt.Println()
	var sb strings.Builder

	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				fmt.Println()
				return sb.String(), calls, false
			}
			if ev.Err != nil {
				fmt.Fprintf(os.Stderr, "\n%serror: %v%s\n", ansiRed, ev.Err, ansiReset)
				return sb.String(), nil, false
			}
			if ev.ToolCall != nil {
				calls = append(calls, *ev.ToolCall)
			}
			if ev.Text != "" {
				fmt.Print(ev.Text)
				sb.WriteString(ev.Text)
			}

		case <-sigCh:
			// User pressed Ctrl+C — cancel the stream and return what we have.
			cancel()
			for range ch {
			}
			fmt.Println()
			return sb.String(), nil, true
		}
	}
}

func historyPath() string {
	home, _ := os.UserHomeDir()
	return home + "/.soulcode/history"
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

const helpText = `
Commands:
  /continue        resume interrupted response
  /model           show current model
  /model <name>    switch to a different model (keeps conversation history)
  /clear           clear conversation history
  /help            show this message
  /exit            exit soulcode  (also Ctrl+D)

Keyboard:
  Ctrl+C           interrupt current generation
  Ctrl+D           exit

`
