package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tanaphonble/soulcode/internal/provider"
	"github.com/tanaphonble/soulcode/internal/security"
)

const maxFileRead = 40 * 1024 // 40 KB — use offset/limit for larger files

func readFileTool() (provider.Tool, executeFn) {
	return provider.Tool{
		Name:        "read_file",
		Description: "Read a file with line numbers. Use offset and limit to read a specific range of lines.",
		Schema: schema(`{
			"type": "object",
			"properties": {
				"path":   {"type": "string", "description": "File path to read."},
				"offset": {"type": "integer", "description": "First line to read (1-based). Omit to start from line 1."},
				"limit":  {"type": "integer", "description": "Maximum number of lines to return. Omit for the whole file."}
			},
			"required": ["path"]
		}`),
	}, runReadFile
}

func runReadFile(_ context.Context, input json.RawMessage, sec *SecurityContext) (string, error) {
	var args struct {
		Path   string `json:"path"`
		Offset int    `json:"offset"`
		Limit  int    `json:"limit"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("read_file: %w", err)
	}
	resolved, err := resolveForRead(args.Path, sec)
	if err != nil {
		return "", fmt.Errorf("read_file: %w", err)
	}
	data, err := os.ReadFile(resolved) //nolint:gosec // path validated by security.ResolveRead
	if err != nil {
		return "", fmt.Errorf("read_file: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	// Remove trailing empty element from a final newline.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	start := 0
	if args.Offset > 1 {
		start = args.Offset - 1
	}
	if start >= len(lines) {
		return fmt.Sprintf("(file has %d lines, offset %d is out of range)", len(lines), args.Offset), nil
	}
	end := len(lines)
	if args.Limit > 0 && start+args.Limit < end {
		end = start + args.Limit
	}

	var sb strings.Builder
	width := len(strconv.Itoa(len(lines)))
	for i, line := range lines[start:end] {
		fmt.Fprintf(&sb, "%*d\t%s\n", width, start+i+1, line)
	}
	if end < len(lines) {
		fmt.Fprintf(&sb, "... (%d lines not shown, use offset=%d to continue)\n", len(lines)-end, end+1)
	}

	result := sb.String()
	if len(result) > maxFileRead {
		result = result[:maxFileRead] + "\n... (truncated, file is large — use offset/limit to read specific sections)"
	}
	return result, nil
}

func writeFileTool() (provider.Tool, executeFn) {
	return provider.Tool{
		Name:        "write_file",
		Description: "Write content to a file, creating it (and parent directories) if needed.",
		Schema: schema(`{
			"type": "object",
			"properties": {
				"path":    {"type": "string", "description": "File path to write."},
				"content": {"type": "string", "description": "Content to write."}
			},
			"required": ["path", "content"]
		}`),
	}, runWriteFile
}

func runWriteFile(_ context.Context, input json.RawMessage, sec *SecurityContext) (string, error) {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("write_file: %w", err)
	}
	resolved, err := resolveForWrite(args.Path, sec)
	if err != nil {
		return "", fmt.Errorf("write_file: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0755); err != nil { //nolint:gosec // 0755 is correct for project directories
		return "", fmt.Errorf("write_file: %w", err)
	}
	if err := os.WriteFile(resolved, []byte(args.Content), 0644); err != nil { //nolint:gosec // 0644 is correct for user project files
		return "", fmt.Errorf("write_file: %w", err)
	}
	gofmt(resolved)
	return fmt.Sprintf("wrote %d bytes to %s", len(args.Content), resolved), nil
}

func editFileTool() (provider.Tool, executeFn) {
	return provider.Tool{
		Name:        "edit_file",
		Description: "Replace a unique string in a file with a new string. old_string must appear exactly once in the file.",
		Schema: schema(`{
			"type": "object",
			"properties": {
				"path":       {"type": "string", "description": "File to edit."},
				"old_string": {"type": "string", "description": "Exact string to replace (must be unique in the file)."},
				"new_string": {"type": "string", "description": "Replacement string."}
			},
			"required": ["path", "old_string", "new_string"]
		}`),
	}, runEditFile
}

func runEditFile(_ context.Context, input json.RawMessage, sec *SecurityContext) (string, error) {
	var args struct {
		Path      string `json:"path"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("edit_file: %w", err)
	}
	resolved, err := resolveForRead(args.Path, sec)
	if err != nil {
		return "", fmt.Errorf("edit_file: %w", err)
	}
	data, err := os.ReadFile(resolved) //nolint:gosec // path validated
	if err != nil {
		return "", fmt.Errorf("edit_file: %w", err)
	}
	content := string(data)
	count := strings.Count(content, args.OldString)
	switch count {
	case 0:
		return "", fmt.Errorf("edit_file: old_string not found in %s", resolved)
	case 1:
		// good
	default:
		return "", fmt.Errorf("edit_file: old_string appears %d times in %s — make it more specific", count, resolved)
	}
	updated := strings.Replace(content, args.OldString, args.NewString, 1)
	if err := os.WriteFile(resolved, []byte(updated), 0644); err != nil { //nolint:gosec // path validated
		return "", fmt.Errorf("edit_file: %w", err)
	}
	gofmt(resolved)
	return fmt.Sprintf("edited %s\n%s", resolved, inlineDiff(args.OldString, args.NewString)), nil
}

// inlineDiff returns a compact before/after diff for display in tool results.
func inlineDiff(oldStr, newStr string) string {
	oldLines := strings.Split(oldStr, "\n")
	newLines := strings.Split(newStr, "\n")
	var sb strings.Builder
	for _, l := range oldLines {
		fmt.Fprintf(&sb, "- %s\n", l)
	}
	for _, l := range newLines {
		fmt.Fprintf(&sb, "+ %s\n", l)
	}
	return sb.String()
}

// resolveForRead applies the workdir boundary if a security context with a
// workdir is supplied. When no workdir is configured (older tests, embedding
// scenarios), the input path is returned unchanged.
func resolveForRead(path string, sec *SecurityContext) (string, error) {
	if sec == nil || sec.Workdir == "" {
		return path, nil
	}
	return security.ResolveRead(path, sec.Workdir)
}

// resolveForWrite mirrors resolveForRead for write-side operations (the file
// itself need not exist yet).
func resolveForWrite(path string, sec *SecurityContext) (string, error) {
	if sec == nil || sec.Workdir == "" {
		return path, nil
	}
	return security.ResolveWrite(path, sec.Workdir)
}

// gofmt runs gofmt -w on .go files silently — best effort, errors ignored.
func gofmt(path string) {
	if strings.HasSuffix(path, ".go") {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = exec.CommandContext(ctx, "gofmt", "-w", path).Run() //nolint:gosec // gofmt fixed cmd, path from write_file
	}
}

func lsTool() (provider.Tool, executeFn) {
	return provider.Tool{
		Name:        "ls",
		Description: "List files and directories at a path.",
		Schema: schema(`{
			"type": "object",
			"properties": {
				"path": {"type": "string", "description": "Directory path to list. Defaults to current directory."}
			}
		}`),
	}, runLs
}

func runLs(_ context.Context, input json.RawMessage, sec *SecurityContext) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("ls: invalid input: %w", err)
	}
	if args.Path == "" {
		args.Path = "."
	}
	resolved, err := resolveForRead(args.Path, sec)
	if err != nil {
		return "", fmt.Errorf("ls: %w", err)
	}
	entries, err := os.ReadDir(resolved)
	if err != nil {
		return "", fmt.Errorf("ls: %w", err)
	}
	var sb strings.Builder
	for _, e := range entries {
		info, _ := e.Info()
		switch {
		case e.IsDir():
			fmt.Fprintf(&sb, "%s/\n", e.Name())
		case info != nil:
			fmt.Fprintf(&sb, "%s (%d bytes)\n", e.Name(), info.Size())
		default:
			fmt.Fprintf(&sb, "%s\n", e.Name())
		}
	}
	return sb.String(), nil
}
