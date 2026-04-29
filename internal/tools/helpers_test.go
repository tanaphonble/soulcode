package tools_test

import (
	"encoding/json"
	"os"
	"testing"

	"soulcode/internal/provider"
	"soulcode/internal/security"
	"soulcode/internal/tools"
)

// call builds a ToolCall for use in tests.
func call(name, input string) provider.ToolCall {
	return provider.ToolCall{ID: "test", Name: name, Input: json.RawMessage(input)}
}

// newRegistry builds a tools.Registry scoped to workdir, with an auto-approver
// (allows everything except dangerous patterns) and no auditor. Use this in
// tests instead of tools.New so the new SecurityContext signature is honoured
// uniformly.
func newRegistry(workdir string) *tools.Registry {
	return tools.New(&tools.SecurityContext{
		Workdir:  workdir,
		Approver: security.AutoApprover{},
	})
}

func mustWriteFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0644); err != nil { //nolint:gosec // test helper writes project files
		t.Fatalf("mustWriteFile %s: %v", path, err)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.Mkdir(path, 0755); err != nil { //nolint:gosec // test helper creates project dirs
		t.Fatalf("mustMkdir %s: %v", path, err)
	}
}
