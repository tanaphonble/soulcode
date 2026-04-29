package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"soulcode/internal/provider"
	"soulcode/internal/security"
)

const maxSearchResults = 200

func grepTool() (provider.Tool, executeFn) {
	return provider.Tool{
		Name:        "grep",
		Description: "Search for a regex pattern in files. Returns matching lines with file:line format.",
		Schema: schema(`{
			"type": "object",
			"properties": {
				"pattern": {"type": "string", "description": "Regular expression to search for."},
				"path":    {"type": "string", "description": "File or directory to search. Defaults to current directory."},
				"glob":    {"type": "string", "description": "Glob pattern to filter files, e.g. '*.go'."}
			},
			"required": ["pattern"]
		}`),
	}, runGrep
}

func runGrep(_ context.Context, input json.RawMessage, sec *SecurityContext) (string, error) {
	var args struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
		Glob    string `json:"glob"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("grep: %w", err)
	}
	if args.Path == "" {
		args.Path = "."
	}

	re, err := regexp.Compile(args.Pattern)
	if err != nil {
		return "", fmt.Errorf("grep: invalid pattern: %w", err)
	}

	root, err := resolveForRead(args.Path, sec)
	if err != nil {
		return "", fmt.Errorf("grep: %w", err)
	}

	var results []string

	err = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if args.Glob != "" {
			matched, _ := filepath.Match(args.Glob, filepath.Base(p))
			if !matched {
				return nil
			}
		}
		// Skip blocked sensitive files inside otherwise-allowed trees.
		if security.IsSensitive(p) {
			return nil
		}
		data, err := os.ReadFile(p) //nolint:gosec // path comes from filepath.Walk under validated root
		if err != nil {
			return nil // skip unreadable files
		}
		for i, line := range strings.Split(string(data), "\n") {
			if re.MatchString(line) {
				results = append(results, fmt.Sprintf("%s:%d: %s", p, i+1, line))
				if len(results) >= maxSearchResults {
					return filepath.SkipAll
				}
			}
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("grep: %w", err)
	}
	if len(results) == 0 {
		return "no matches found", nil
	}
	out := strings.Join(results, "\n")
	if len(results) >= maxSearchResults {
		out += fmt.Sprintf("\n... (showing first %d matches)", maxSearchResults)
	}
	return out, nil
}

func globTool() (provider.Tool, executeFn) {
	return provider.Tool{
		Name:        "glob",
		Description: "Find files matching a glob pattern. Returns a list of matching paths.",
		Schema: schema(`{
			"type": "object",
			"properties": {
				"pattern": {"type": "string", "description": "Glob pattern, e.g. 'src/**/*.go'."},
				"path":    {"type": "string", "description": "Root directory to search. Defaults to current directory."}
			},
			"required": ["pattern"]
		}`),
	}, runGlob
}

func runGlob(_ context.Context, input json.RawMessage, sec *SecurityContext) (string, error) {
	var args struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("glob: %w", err)
	}
	if args.Path == "" {
		args.Path = "."
	}

	root, err := resolveForRead(args.Path, sec)
	if err != nil {
		return "", fmt.Errorf("glob: %w", err)
	}

	var matches []string
	err = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if security.IsSensitive(p) {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		matched, _ := filepath.Match(args.Pattern, rel)
		if !matched {
			// also try matching just the filename
			matched, _ = filepath.Match(args.Pattern, filepath.Base(p))
		}
		if matched {
			matches = append(matches, p)
		}
		if len(matches) >= maxSearchResults {
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("glob: %w", err)
	}
	if len(matches) == 0 {
		return "no files found", nil
	}
	return strings.Join(matches, "\n"), nil
}
