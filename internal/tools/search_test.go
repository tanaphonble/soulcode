package tools_test

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestGrep_FindsMatches(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "a.go"), []byte("func main() {}\n"))
	mustWriteFile(t, filepath.Join(dir, "b.go"), []byte("func helper() {}\n"))

	reg := newRegistry(dir)
	out, err := reg.Execute(context.Background(), call("grep", fmt.Sprintf(`{"pattern":"func","path":%q}`, dir)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "a.go") || !strings.Contains(out, "b.go") {
		t.Errorf("expected both files in output, got %q", out)
	}
}

func TestGrep_NoMatches(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "a.go"), []byte("package main\n"))

	reg := newRegistry(dir)
	out, err := reg.Execute(context.Background(), call("grep", fmt.Sprintf(`{"pattern":"NOTFOUND","path":%q}`, dir)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "no matches") {
		t.Errorf("expected 'no matches', got %q", out)
	}
}

func TestGrep_InvalidRegex(t *testing.T) {
	t.Parallel()
	reg := newRegistry(t.TempDir())

	_, err := reg.Execute(context.Background(), call("grep", `{"pattern":"[invalid"}`))
	if err == nil {
		t.Error("expected error for invalid regex")
	}
}

func TestGrep_GlobFilter(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "main.go"), []byte("package main\n"))
	mustWriteFile(t, filepath.Join(dir, "main.ts"), []byte("const x = 1\n"))

	reg := newRegistry(dir)
	out, err := reg.Execute(context.Background(), call("grep", fmt.Sprintf(
		`{"pattern":"main","path":%q,"glob":"*.go"}`, dir,
	)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(out, ".ts") {
		t.Error("glob filter should have excluded .ts files")
	}
}

func TestGlob_FindsFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "main.go"), []byte(""))
	mustWriteFile(t, filepath.Join(dir, "main_test.go"), []byte(""))
	mustWriteFile(t, filepath.Join(dir, "README.md"), []byte(""))

	reg := newRegistry(dir)
	out, err := reg.Execute(context.Background(), call("glob", fmt.Sprintf(`{"pattern":"*.go","path":%q}`, dir)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "main.go") {
		t.Errorf("expected main.go in output, got %q", out)
	}
	if strings.Contains(out, "README.md") {
		t.Error("README.md should not match *.go")
	}
}

func TestGlob_NoMatches(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "main.go"), []byte(""))

	reg := newRegistry(dir)
	out, err := reg.Execute(context.Background(), call("glob", fmt.Sprintf(`{"pattern":"*.ts","path":%q}`, dir)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "no files") {
		t.Errorf("expected 'no files', got %q", out)
	}
}
