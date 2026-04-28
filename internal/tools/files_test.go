package tools_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"soulcode/internal/tools"
)

func TestReadFile_Success(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "hello.txt")
	mustWriteFile(t, path, []byte("hello world"))

	reg := tools.New()
	out, err := reg.Execute(context.Background(), call("read_file", fmt.Sprintf(`{"path":%q}`, path)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "hello world" {
		t.Errorf("expected 'hello world', got %q", out)
	}
}

func TestReadFile_NotFound(t *testing.T) {
	t.Parallel()
	reg := tools.New()

	_, err := reg.Execute(context.Background(), call("read_file", `{"path":"/nonexistent/file.txt"}`))
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestWriteFile_CreatesFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "new.txt")

	reg := tools.New()
	_, err := reg.Execute(context.Background(), call("write_file", fmt.Sprintf(`{"path":%q,"content":"content"}`, path)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := os.ReadFile(path) //nolint:gosec // test reads its own temp file
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	if string(data) != "content" {
		t.Errorf("unexpected content: %q", data)
	}
}

func TestWriteFile_Overwrite(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	mustWriteFile(t, path, []byte("old"))

	reg := tools.New()
	if _, err := reg.Execute(context.Background(), call("write_file", fmt.Sprintf(`{"path":%q,"content":"new"}`, path))); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(path) //nolint:gosec // test reads its own temp file
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(data) != "new" {
		t.Errorf("expected 'new', got %q", data)
	}
}

func TestEditFile_Success(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "code.go")
	mustWriteFile(t, path, []byte("func hello() {}\n"))

	reg := tools.New()
	_, err := reg.Execute(context.Background(), call("edit_file", fmt.Sprintf(
		`{"path":%q,"old_string":"func hello()","new_string":"func goodbye()"}`, path,
	)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := os.ReadFile(path) //nolint:gosec // test reads its own temp file
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !strings.Contains(string(data), "func goodbye()") {
		t.Errorf("edit not applied: %q", data)
	}
}

func TestEditFile_NotFound(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "f.go")
	mustWriteFile(t, path, []byte("package main\n"))

	reg := tools.New()
	_, err := reg.Execute(context.Background(), call("edit_file", fmt.Sprintf(
		`{"path":%q,"old_string":"missing","new_string":"x"}`, path,
	)))
	if err == nil {
		t.Error("expected error when old_string not found")
	}
}

func TestEditFile_Ambiguous(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "f.go")
	mustWriteFile(t, path, []byte("foo\nfoo\n"))

	reg := tools.New()
	_, err := reg.Execute(context.Background(), call("edit_file", fmt.Sprintf(
		`{"path":%q,"old_string":"foo","new_string":"bar"}`, path,
	)))
	if err == nil {
		t.Error("expected error when old_string appears multiple times")
	}
}

func TestLs_ListsEntries(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "a.txt"), []byte("x"))
	mustMkdir(t, filepath.Join(dir, "subdir"))

	reg := tools.New()
	out, err := reg.Execute(context.Background(), call("ls", fmt.Sprintf(`{"path":%q}`, dir)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "a.txt") {
		t.Errorf("expected a.txt in output, got %q", out)
	}
	if !strings.Contains(out, "subdir/") {
		t.Errorf("expected subdir/ in output, got %q", out)
	}
}

func TestLs_DefaultsToCurrentDir(t *testing.T) {
	t.Parallel()
	reg := tools.New()
	out, err := reg.Execute(context.Background(), call("ls", `{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == "" {
		t.Error("expected non-empty output for current directory")
	}
}
