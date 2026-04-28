package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"soulcode/internal/provider"
)

const maxFileRead = 100 * 1024 // 100 KB

func readFileTool() (provider.Tool, executeFn) {
	return provider.Tool{
		Name:        "read_file",
		Description: "Read the contents of a file at the given path.",
		Schema: schema(`{
			"type": "object",
			"properties": {
				"path": {"type": "string", "description": "File path to read."}
			},
			"required": ["path"]
		}`),
	}, runReadFile
}

func runReadFile(_ context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("read_file: %w", err)
	}
	data, err := os.ReadFile(args.Path)
	if err != nil {
		return "", fmt.Errorf("read_file: %w", err)
	}
	content := string(data)
	if len(content) > maxFileRead {
		content = content[:maxFileRead] + fmt.Sprintf("\n... (truncated, %d bytes total)", len(data))
	}
	return content, nil
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

func runWriteFile(_ context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("write_file: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(args.Path), 0755); err != nil { //nolint:gosec // 0755 is correct for project directories
		return "", fmt.Errorf("write_file: %w", err)
	}
	if err := os.WriteFile(args.Path, []byte(args.Content), 0644); err != nil { //nolint:gosec // 0644 is correct for user project files
		return "", fmt.Errorf("write_file: %w", err)
	}
	gofmt(args.Path)
	return fmt.Sprintf("wrote %d bytes to %s", len(args.Content), args.Path), nil
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

func runEditFile(_ context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Path      string `json:"path"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("edit_file: %w", err)
	}
	data, err := os.ReadFile(args.Path)
	if err != nil {
		return "", fmt.Errorf("edit_file: %w", err)
	}
	content := string(data)
	count := strings.Count(content, args.OldString)
	switch count {
	case 0:
		return "", fmt.Errorf("edit_file: old_string not found in %s", args.Path)
	case 1:
		// good
	default:
		return "", fmt.Errorf("edit_file: old_string appears %d times in %s — make it more specific", count, args.Path)
	}
	updated := strings.Replace(content, args.OldString, args.NewString, 1)
	if err := os.WriteFile(args.Path, []byte(updated), 0644); err != nil { //nolint:gosec // 0644 correct for project files
		return "", fmt.Errorf("edit_file: %w", err)
	}
	gofmt(args.Path)
	return fmt.Sprintf("edited %s", args.Path), nil
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

func runLs(_ context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("ls: invalid input: %w", err)
	}
	if args.Path == "" {
		args.Path = "."
	}
	entries, err := os.ReadDir(args.Path)
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
