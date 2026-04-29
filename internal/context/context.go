// Package context gathers project context injected into the system prompt.
package context

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tanaphonble/soulcode/internal/repomap"
)

// Project holds everything soulcode knows about the working environment.
type Project struct {
	WorkDir     string
	GitRoot     string
	GitBranch   string
	GitStatus   string
	SoulcodeDoc string
	RepoMap     string
}

// Gather collects project context from the current working directory.
// Errors are silently ignored — partial context is better than no context.
func Gather() *Project {
	p := &Project{}
	p.WorkDir, _ = os.Getwd()
	p.GitRoot = gitRoot()
	p.GitBranch = gitBranch()
	p.GitStatus = gitStatus()
	p.SoulcodeDoc = findSoulcodeDoc(p.WorkDir)

	// Build repo map from git root if available, otherwise working directory.
	mapRoot := p.GitRoot
	if mapRoot == "" {
		mapRoot = p.WorkDir
	}
	p.RepoMap = repomap.Build(mapRoot)
	return p
}

// SystemPrompt builds the full system prompt by combining the base prompt
// with the gathered project context.
func (p *Project) SystemPrompt(base string) string {
	var sb strings.Builder
	sb.WriteString(base)
	sb.WriteString("\n\n# Environment\n")
	fmt.Fprintf(&sb, "Working directory: %s\n", p.WorkDir)

	if p.GitRoot != "" {
		fmt.Fprintf(&sb, "Git root: %s\n", p.GitRoot)
	}
	if p.GitBranch != "" {
		fmt.Fprintf(&sb, "Git branch: %s\n", p.GitBranch)
	}
	if p.GitStatus != "" {
		sb.WriteString("Git status:\n")
		for _, line := range strings.Split(strings.TrimSpace(p.GitStatus), "\n") {
			fmt.Fprintf(&sb, "  %s\n", line)
		}
	}

	if p.SoulcodeDoc != "" {
		sb.WriteString("\n# soulcode.md\n")
		sb.WriteString(p.SoulcodeDoc)
	}

	if p.RepoMap != "" {
		sb.WriteString("\n")
		sb.WriteString(p.RepoMap)
	}

	return sb.String()
}

// ── Git helpers ──────────────────────────────────────────────────────────────

func gitRoot() string {
	out, err := git("rev-parse", "--show-toplevel")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func gitBranch() string {
	out, err := git("branch", "--show-current")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func gitStatus() string {
	out, err := git("status", "--short")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func git(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var buf bytes.Buffer
	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec // git is a fixed command; args are internal constants
	cmd.Stdout = &buf
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// ── soulcode.md discovery ────────────────────────────────────────────────────

// findSoulcodeDoc walks up from dir looking for soulcode.md, stopping at the
// git root or filesystem root. Returns the file contents or empty string.
func findSoulcodeDoc(dir string) string {
	root := gitRoot()
	for {
		path := filepath.Join(dir, "soulcode.md")
		if data, err := os.ReadFile(path); err == nil { //nolint:gosec // path is constructed from trusted workdir + fixed filename
			return string(data)
		}
		parent := filepath.Dir(dir)
		if parent == dir || dir == root {
			break
		}
		dir = parent
	}
	return ""
}
