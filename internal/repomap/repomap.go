// Package repomap builds a compact symbol map of a codebase and injects it
// into the system prompt so the LLM understands what exists without reading
// every file in full.
package repomap

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// maxBytes caps the repo map size to keep it within a reasonable token budget.
// ~12 KB ≈ 3 000 tokens — enough context without crowding the conversation.
const maxBytes = 12_000

// skipDirs are directories that never contain user code.
var skipDirs = map[string]bool{
	".git": true, "vendor": true, "node_modules": true,
	"dist": true, "build": true, ".cache": true,
	"__pycache__": true, ".next": true, "target": true,
}

// Build walks root and returns a compact symbol map of the codebase.
// Returns an empty string if root contains nothing meaningful.
func Build(root string) string {
	entries := collectEntries(root)
	if len(entries) == 0 {
		return ""
	}

	// Sort: non-test files first, then test files, then by path.
	sort.Slice(entries, func(i, j int) bool {
		ti := strings.HasSuffix(entries[i].rel, "_test.go")
		tj := strings.HasSuffix(entries[j].rel, "_test.go")
		if ti != tj {
			return !ti
		}
		return entries[i].rel < entries[j].rel
	})

	var sb strings.Builder
	fmt.Fprintf(&sb, "# Repo Map — %s\n", filepath.Base(root))
	fmt.Fprintf(&sb, "# Compact symbol index. Use read_file for full content.\n\n")

	for _, e := range entries {
		if sb.Len()+len(e.body) > maxBytes {
			fmt.Fprintf(&sb, "# ... (%d more files not shown)\n", len(entries))
			break
		}
		sb.WriteString(e.body)
	}

	return sb.String()
}

type entry struct {
	rel  string
	body string
}

func collectEntries(root string) []entry {
	var entries []entry
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if skipDirs[name] || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		body := buildEntry(p, rel)
		if body != "" {
			entries = append(entries, entry{rel: rel, body: body})
		}
		return nil
	})
	return entries
}

func buildEntry(path, rel string) string {
	switch filepath.Ext(path) {
	case ".go":
		return goEntry(path, rel)
	case ".py":
		return regexEntry(path, rel, pyPattern)
	case ".ts", ".tsx":
		return regexEntry(path, rel, tsPattern)
	case ".js", ".jsx":
		return regexEntry(path, rel, jsPattern)
	case ".rs":
		return regexEntry(path, rel, rustPattern)
	default:
		return ""
	}
}
