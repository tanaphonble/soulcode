package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tanaphonble/soulcode/internal/provider"
	"github.com/tanaphonble/soulcode/internal/security"
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

	results, err := walkGrep(root, args.Glob, re)
	if err != nil {
		return "", fmt.Errorf("grep: %w", err)
	}
	return formatGrepResults(results), nil
}

func walkGrep(root, glob string, re *regexp.Regexp) ([]string, error) {
	var results []string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if !grepShouldScan(p, glob) {
			return nil
		}
		hits, err := grepFile(p, re, maxSearchResults-len(results))
		if err != nil {
			return nil // skip unreadable
		}
		results = append(results, hits...)
		if len(results) >= maxSearchResults {
			return filepath.SkipAll
		}
		return nil
	})
	return results, err
}

// grepShouldScan filters out files based on the user's glob and the security
// blocklist before any read happens.
func grepShouldScan(path, glob string) bool {
	if glob != "" {
		if matched, _ := filepath.Match(glob, filepath.Base(path)); !matched {
			return false
		}
	}
	return !security.IsSensitive(path)
}

// grepFile returns up to budget matches from one file.
func grepFile(path string, re *regexp.Regexp, budget int) ([]string, error) {
	if budget <= 0 {
		return nil, nil
	}
	data, err := os.ReadFile(path) //nolint:gosec // path comes from filepath.Walk under validated root
	if err != nil {
		return nil, err
	}
	var hits []string
	for i, line := range strings.Split(string(data), "\n") {
		if re.MatchString(line) {
			hits = append(hits, fmt.Sprintf("%s:%d: %s", path, i+1, line))
			if len(hits) >= budget {
				break
			}
		}
	}
	return hits, nil
}

func formatGrepResults(results []string) string {
	if len(results) == 0 {
		return "no matches found"
	}
	out := strings.Join(results, "\n")
	if len(results) >= maxSearchResults {
		out += fmt.Sprintf("\n... (showing first %d matches)", maxSearchResults)
	}
	return out
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
