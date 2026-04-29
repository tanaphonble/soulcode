// Package ui renders the agent loop's tool calls and approval prompts in a
// compact, structured format. Output goes to stdout via plain ANSI escape
// codes — no TUI framework dependency, in keeping with soulcode's
// zero-runtime philosophy.
package ui

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ANSI escape codes shared across the package and the REPL.
const (
	Reset  = "\033[0m"
	Bold   = "\033[1m"
	Dim    = "\033[2m"
	Italic = "\033[3m"
	Red    = "\033[31m"
	Green  = "\033[32m"
	Yellow = "\033[33m"
	Cyan   = "\033[36m"
)

// Glyph constants, kept ASCII-friendly enough to render in any terminal.
const (
	GlyphRunning = "●"
	GlyphOK      = "✓"
	GlyphFail    = "✗"
	GlyphPrompt  = "?"
	GlyphWarn    = "⚠"
)

// Renderer holds context shared across one REPL session: the working
// directory (for path shortening) and the user's home directory (for the
// `~/...` shorthand). Output goes to its writer, which defaults to os.Stdout.
type Renderer struct {
	WorkDir string
	Home    string
	Out     io.Writer
}

// New returns a Renderer rooted at workDir.
func New(workDir string) *Renderer {
	home, _ := os.UserHomeDir()
	return &Renderer{WorkDir: workDir, Home: home, Out: os.Stdout}
}

// ToolCall is a single in-flight tool invocation. Lifecycle:
//
//	tc := r.ToolStart("bash", input)
//	// ... run tool ...
//	tc.Done(result, err)
func (r *Renderer) ToolStart(name string, input json.RawMessage) *ToolCall {
	summary := summarize(name, input, r.WorkDir, r.Home)
	tc := &ToolCall{name: name, summary: summary, start: time.Now(), r: r}
	tc.printHeader()
	return tc
}

// ToolCall tracks the state of a single tool invocation for layout purposes.
type ToolCall struct {
	name    string
	summary string
	start   time.Time
	r       *Renderer
}

// printf is a small wrapper that swallows the Writer's error. Errors writing
// to stdout are not actionable for an interactive REPL.
func (tc *ToolCall) printf(format string, a ...any) {
	_, _ = fmt.Fprintf(tc.r.Out, format, a...)
}

// Header is "● <name>  <summary>".
func (tc *ToolCall) printHeader() {
	tc.printf("\n%s%s%s %s%s%s",
		Cyan, GlyphRunning, Reset,
		Bold, tc.name, Reset)
	if tc.summary != "" {
		tc.printf("  %s%s%s", Dim, tc.summary, Reset)
	}
	tc.printf("\n")
}

// Done renders the body and the trailing status line.
func (tc *ToolCall) Done(result string, err error) {
	dur := time.Since(tc.start)
	if err != nil {
		tc.printError(result, err, dur)
		return
	}
	body := tc.formatBody(result)
	if body != "" {
		tc.printBody(body)
	}
	tc.printf("  %s%s %s%s\n", Green, GlyphOK, formatDuration(dur), Reset)
}

func (tc *ToolCall) printError(result string, err error, dur time.Duration) {
	body := strings.TrimSpace(result)
	if body == "" {
		body = err.Error()
	}
	body = ShortenPaths(body, tc.r.WorkDir, tc.r.Home)
	body = CollapseLines(body, 5)
	for line := range strings.SplitSeq(strings.TrimRight(body, "\n"), "\n") {
		tc.printf("  %s%s%s\n", Red, line, Reset)
	}
	tc.printf("  %s%s %s%s\n", Red, GlyphFail, formatDuration(dur), Reset)
}

// formatBody applies tool-specific transforms (diff colouring for edit_file)
// and the global path shortening + line collapsing.
func (tc *ToolCall) formatBody(result string) string {
	body := result
	if tc.name == "edit_file" {
		body = ColourDiff(body)
	}
	body = ShortenPaths(body, tc.r.WorkDir, tc.r.Home)
	return CollapseLines(body, 5)
}

func (tc *ToolCall) printBody(body string) {
	for line := range strings.SplitSeq(strings.TrimRight(body, "\n"), "\n") {
		tc.printf("  %s%s%s\n", Dim, line, Reset)
	}
}

// formatDuration returns a compact human-friendly duration: 0.3s, 12s, 1m04s.
func formatDuration(d time.Duration) string {
	switch {
	case d < time.Second:
		return fmt.Sprintf("%.1fs", d.Seconds())
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	default:
		m := int(d.Minutes())
		s := int(d.Seconds()) - m*60
		return fmt.Sprintf("%dm%02ds", m, s)
	}
}

// ShortenPath returns a compact form of an absolute path: relative to workdir
// when it lives inside (and "." when it equals workdir), "~/..." when it
// lives under the user's home but outside workdir, or the original input as
// a last resort.
func ShortenPath(p, workDir, home string) string {
	if !filepath.IsAbs(p) {
		return p
	}
	if workDir != "" {
		if rel, err := filepath.Rel(workDir, p); err == nil && !strings.HasPrefix(rel, "..") {
			return rel
		}
	}
	if home != "" && (p == home || strings.HasPrefix(p, home+string(filepath.Separator))) {
		return "~" + p[len(home):]
	}
	return p
}

// pathLikePattern matches things that look like absolute POSIX paths so we
// can rewrite them in tool output. We keep it simple: a leading slash, then
// a run of non-whitespace, non-quote characters.
var pathLikeBoundary = func(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r', '"', '\'', '(', ')', '[', ']', '{', '}', ',', ':', ';':
		return true
	}
	return false
}

// ShortenPaths walks s and rewrites each absolute path it can identify into
// the compact form produced by ShortenPath. Non-paths pass through untouched.
func ShortenPaths(s, workDir, home string) string {
	if s == "" || (workDir == "" && home == "") {
		return s
	}
	var sb strings.Builder
	sb.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] != '/' {
			sb.WriteByte(s[i])
			i++
			continue
		}
		// Find end of this path token.
		j := i
		for j < len(s) && !pathLikeBoundary(rune(s[j])) {
			j++
		}
		token := s[i:j]
		// Strip a trailing punctuation like "." that often follows a path in prose.
		trailing := ""
		for len(token) > 0 {
			last := token[len(token)-1]
			if last == '.' || last == ',' || last == ':' || last == ';' {
				trailing = string(last) + trailing
				token = token[:len(token)-1]
				continue
			}
			break
		}
		short := ShortenPath(token, workDir, home)
		sb.WriteString(short)
		sb.WriteString(trailing)
		i = j
	}
	return sb.String()
}

// CollapseLines truncates s to at most maxLines lines, replacing the tail
// with a "… N more lines" marker that styling code may or may not colour.
func CollapseLines(s string, maxLines int) string {
	if maxLines <= 0 || s == "" {
		return s
	}
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= maxLines {
		return strings.Join(lines, "\n")
	}
	head := lines[:maxLines-1]
	hidden := len(lines) - len(head)
	return strings.Join(head, "\n") + fmt.Sprintf("\n%s… %d more lines%s", Italic, hidden, Reset)
}

// ColourDiff applies green/red ANSI to each line that begins with "+"/"-"
// (preserving the marker character). Other lines pass through unchanged.
func ColourDiff(s string) string {
	if s == "" {
		return s
	}
	var sb strings.Builder
	sb.Grow(len(s))
	for i, line := range strings.Split(s, "\n") {
		if i > 0 {
			sb.WriteByte('\n')
		}
		switch {
		case strings.HasPrefix(line, "+"):
			sb.WriteString(Green)
			sb.WriteString(line)
			sb.WriteString(Reset)
		case strings.HasPrefix(line, "-"):
			sb.WriteString(Red)
			sb.WriteString(line)
			sb.WriteString(Reset)
		default:
			sb.WriteString(line)
		}
	}
	return sb.String()
}

// summarize extracts a one-line description of a tool call from its JSON
// input. Used as the right-hand side of "● <name>  <summary>".
func summarize(name string, input json.RawMessage, workDir, home string) string {
	switch name {
	case "bash":
		return summarizeBash(input)
	case "read_file":
		return summarizePath(input, workDir, home)
	case "write_file":
		return summarizeWrite(input, workDir, home)
	case "edit_file":
		return summarizePath(input, workDir, home)
	case "ls":
		return summarizeLS(input, workDir, home)
	case "grep":
		return summarizeGrep(input, workDir, home)
	case "glob":
		return summarizeGlob(input, workDir, home)
	case "think":
		return ""
	}
	return ""
}

func summarizeBash(input json.RawMessage) string {
	var a struct {
		Command string `json:"command"`
	}
	_ = json.Unmarshal(input, &a)
	return TruncateOneLine(a.Command, 100)
}

func summarizePath(input json.RawMessage, workDir, home string) string {
	var a struct {
		Path   string `json:"path"`
		Offset int    `json:"offset"`
		Limit  int    `json:"limit"`
	}
	_ = json.Unmarshal(input, &a)
	short := ShortenPath(a.Path, workDir, home)
	if a.Offset > 0 || a.Limit > 0 {
		return fmt.Sprintf("%s @%d+%d", short, a.Offset, a.Limit)
	}
	return short
}

func summarizeWrite(input json.RawMessage, workDir, home string) string {
	var a struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	_ = json.Unmarshal(input, &a)
	return fmt.Sprintf("%s (%d bytes)", ShortenPath(a.Path, workDir, home), len(a.Content))
}

func summarizeLS(input json.RawMessage, workDir, home string) string {
	var a struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal(input, &a)
	if a.Path == "" {
		return "."
	}
	return ShortenPath(a.Path, workDir, home)
}

func summarizeGrep(input json.RawMessage, workDir, home string) string {
	var a struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
		Glob    string `json:"glob"`
	}
	_ = json.Unmarshal(input, &a)
	path := a.Path
	if path == "" {
		path = "."
	}
	out := fmt.Sprintf("%q in %s", a.Pattern, ShortenPath(path, workDir, home))
	if a.Glob != "" {
		out += " (" + a.Glob + ")"
	}
	return out
}

func summarizeGlob(input json.RawMessage, workDir, home string) string {
	var a struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
	}
	_ = json.Unmarshal(input, &a)
	if a.Path == "" {
		return a.Pattern
	}
	return fmt.Sprintf("%s in %s", a.Pattern, ShortenPath(a.Path, workDir, home))
}

// TruncateOneLine collapses any embedded newline to a space and caps the
// total length to n runes (with a … marker when truncated).
func TruncateOneLine(s string, n int) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		s = s[:idx] + " " + strings.ReplaceAll(s[idx+1:], "\n", " ")
	}
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
