package security

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// AllowList is a set of bash command prefixes that have been pre-approved for
// the project. It is loaded from <workdir>/.soulcode/allow.txt — one prefix
// per line, comments start with "#".
//
// Matching is prefix-based (after trimming leading whitespace) so a line
// "go test ./..." auto-approves any command that starts with that exact text.
type AllowList struct {
	prefixes []string
}

// LoadAllowList reads <workdir>/.soulcode/allow.txt. A missing file produces
// an empty AllowList; parse errors fall back to the same.
func LoadAllowList(workdir string) *AllowList {
	path := filepath.Join(workdir, ".soulcode", "allow.txt")
	f, err := os.Open(path) //nolint:gosec // path scoped to workdir
	if err != nil {
		return &AllowList{}
	}
	defer func() { _ = f.Close() }()

	var prefixes []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		prefixes = append(prefixes, line)
	}
	return &AllowList{prefixes: prefixes}
}

// Allows reports whether cmd starts with any allowed prefix. Dangerous
// commands are NEVER auto-approved by the allow list — that decision lives in
// the Approver.
func (a *AllowList) Allows(cmd string) bool {
	if a == nil {
		return false
	}
	cmd = strings.TrimSpace(cmd)
	for _, p := range a.prefixes {
		if strings.HasPrefix(cmd, p) {
			return true
		}
	}
	return false
}
