// Package repl implements the interactive read-eval-print loop.
package repl

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/chzyer/readline"

	sctx "github.com/tanaphonble/soulcode/internal/context"
	"github.com/tanaphonble/soulcode/internal/provider"
	"github.com/tanaphonble/soulcode/internal/security"
	"github.com/tanaphonble/soulcode/internal/session"
	"github.com/tanaphonble/soulcode/internal/tools"
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
	p          provider.Provider
	switchFn   SwitchFn
	sess       *session.Session
	tools      *tools.Registry // built in Run() once readline is wired
	workDir    string
	sessionKey string // either "auto-<hash>" or a user-supplied name
	restored   bool
	yolo       bool
	allowList  *security.AllowList
	auditor    *security.Auditor
}

const systemPrompt = `You are soulcode, a world-class terminal-based software engineering assistant.

## Mandatory rules

1. **Clarify first** — for non-trivial tasks, ask one short round of questions before writing any code.
2. **Read before editing** — always read_file before editing. Use grep/glob to understand context.
3. **Verify every change** — after every write_file or edit_file, run bash (build + tests). Fix all errors before continuing. Never report success while errors exist.
4. **No placeholders** — write complete, production-ready code. No TODOs, stubs, or truncated output.
5. **Match the codebase** — use existing patterns, style, naming, and error-handling conventions exactly.
6. **Handle errors** — never silently discard errors.
7. **Dependencies** — after adding any: go mod tidy / npm install / pip install.
8. **Prefer edit_file** over write_file for existing files — smaller diffs, lower risk.

## Output
No narration. Do the work. End with one short summary of what changed and how to run it.
Mid-task ambiguity: decide and note it in one sentence.`

// Options configures behaviour of a REPL beyond the required provider/session
// wiring. Zero values are safe defaults.
type Options struct {
	// SessionName, when non-empty, overrides the default per-directory auto
	// session and uses a named session shared across directories.
	SessionName string
	// Yolo skips the per-command bash approval prompt. Dangerous patterns
	// (rm -rf /, curl|sh, etc.) still require confirmation.
	Yolo bool
}

// New creates a REPL backed by the given provider.
// switchFn is called when the user runs /model <name>.
func New(p provider.Provider, switchFn SwitchFn, opts Options) *REPL {
	proj := sctx.Gather()
	sess := session.New(proj.SystemPrompt(systemPrompt))

	key := session.AutoKey(proj.WorkDir)
	if opts.SessionName != "" {
		key = opts.SessionName
	}
	restored, _ := sess.Load(key)

	return &REPL{
		p:          p,
		switchFn:   switchFn,
		sess:       sess,
		workDir:    proj.WorkDir,
		sessionKey: key,
		restored:   restored,
		yolo:       opts.Yolo || security.EnvYolo(),
		allowList:  security.LoadAllowList(proj.WorkDir),
		auditor:    security.NewAuditor(),
	}
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
	defer func() { _ = rl.Close() }()

	fmt.Printf("%ssoulcode%s  %s%s%s\n", ansiBold, ansiReset, ansiDim, r.p.ID(), ansiReset)
	fmt.Printf("%sCtrl+C%s interrupts · %sCtrl+D%s exits · %s/help%s for commands\n",
		ansiBold, ansiReset, ansiBold, ansiReset, ansiBold, ansiReset)
	if r.yolo {
		fmt.Printf("%s[yolo mode: bash auto-approves except dangerous commands]%s\n", ansiYellow, ansiReset)
	}
	if r.restored {
		label := r.sessionKey
		fmt.Printf("%sSession %q restored (%d messages) — /clear to start fresh%s\n",
			ansiDim, label, r.sess.Len(), ansiReset)
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

	// Wire the security context now that readline is alive. The approver
	// reuses lineCh so it never competes with the main input goroutine for
	// stdin.
	approver := security.NewReadlineApprover(rl, lineCh, promptStr, r.yolo)
	r.tools = tools.New(&tools.SecurityContext{
		Workdir:   r.workDir,
		Approver:  approver,
		AllowList: r.allowList,
		Auditor:   r.auditor,
	})

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
	if err := r.sess.Save(r.sessionKey); err != nil {
		fmt.Fprintf(os.Stderr, "%s[warning: session not saved: %v]%s\n", ansiYellow, err, ansiReset)
	}
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
		r.sess.DeleteSaved(r.sessionKey)
		fmt.Printf("%sConversation cleared.%s\n", ansiDim, ansiReset)

	case "/continue":
		r.agentLoop(ctx, "Continue your previous response from where you left off.", sigCh)

	case "/model":
		r.handleModel(parts[1:])

	case "/sessions":
		r.handleSessions()

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

// handleSessions lists all named sessions.
func (r *REPL) handleSessions() {
	sessions, err := session.ListNamed()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%serror listing sessions: %v%s\n", ansiRed, err, ansiReset)
		return
	}
	if len(sessions) == 0 {
		fmt.Printf("%sNo named sessions yet. Start one with: soulcode -s <name>%s\n", ansiDim, ansiReset)
		return
	}
	fmt.Printf("%s%-24s  %-8s  %s%s\n", ansiBold, "NAME", "MESSAGES", "LAST SAVED", ansiReset)
	for _, s := range sessions {
		active := "  "
		if s.Key == r.sessionKey {
			active = ansiGreen + "* " + ansiReset
		}
		age := formatAge(s.SavedAt)
		fmt.Printf("%s%s%-24s  %-8d  %s%s\n", active, ansiDim, s.Name, s.Count, age, ansiReset)
	}
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
			if call.Name != "think" {
				fmt.Printf("\n%s[%s]%s\n", ansiCyan, call.Name, ansiReset)
			}
			result, err := r.tools.Execute(ctx, call)
			if err != nil {
				result = fmt.Sprintf("error: %v", err)
				fmt.Printf("%s%s%s\n", ansiRed, result, ansiReset)
			} else if call.Name != "think" {
				fmt.Printf("%s%s%s\n", ansiDim, truncate(result, 400), ansiReset)
			}
			// Scrub well-known credential shapes before the result is sent
			// back to the LLM. The user is warned in the terminal so they
			// know a leak was caught locally.
			redacted, hits := security.Redact(result)
			if len(hits) > 0 {
				fmt.Printf("%s[secrets redacted before sending to model: %s]%s\n",
					ansiYellow, security.SummariseHits(hits), ansiReset)
				result = redacted
			}
			r.audit(call, result, err, hits)
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

	ch, err := r.p.Chat(streamCtx, r.sess.MessagesForAPI(600, 4), r.tools.Definitions())
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

// audit appends a row to ~/.soulcode/audit.log for every tool call other
// than "bash" (whose audit row carries an approval label written from the
// bash tool itself) and "think" (whose effects are model-side only).
func (r *REPL) audit(call provider.ToolCall, result string, err error, hits []string) {
	if r.auditor == nil || call.Name == "bash" || call.Name == "think" {
		return
	}
	entry := security.AuditEntry{
		Tool:       call.Name,
		Args:       security.FormatArgs(call.Input),
		OK:         err == nil,
		ResultHash: security.HashResult(result),
		SecretHits: hits,
	}
	if err != nil {
		entry.Err = err.Error()
	}
	r.auditor.Log(entry)
}

func historyPath() string {
	home, _ := os.UserHomeDir()
	path := home + "/.soulcode/history"
	// Tighten permissions: history can contain prompts that include secrets.
	// Best effort — readline creates the file if missing, so chmod after the
	// first session opens it.
	_ = os.Chmod(path, 0600)
	return path
}

func formatAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
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
  /sessions        list all named sessions
  /clear           clear conversation history
  /help            show this message
  /exit            exit soulcode  (also Ctrl+D)

Keyboard:
  Ctrl+C           interrupt current generation
  Ctrl+D           exit

Sessions:
  soulcode -s <name>   start or resume a named session
  soulcode             auto-session keyed to current directory

`
