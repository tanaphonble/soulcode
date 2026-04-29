package security

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveRead_AcceptsPathInsideWorkdir(t *testing.T) {
	t.Parallel()
	wd := t.TempDir()
	target := filepath.Join(wd, "sub", "file.txt")
	mustMkdirAll(t, filepath.Dir(target))
	mustWriteFile(t, target, "x")

	got, err := ResolveRead("sub/file.txt", wd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantReal, _ := filepath.EvalSymlinks(target)
	if got != wantReal && got != target {
		t.Errorf("got %q, want %q (or %q)", got, wantReal, target)
	}
}

func TestResolveRead_RejectsOutsideWorkdir(t *testing.T) {
	t.Parallel()
	wd := t.TempDir()
	outside := filepath.Join(t.TempDir(), "elsewhere.txt")
	mustWriteFile(t, outside, "x")

	if _, err := ResolveRead(outside, wd); err == nil {
		t.Error("expected error reading outside workdir")
	}
	if _, err := ResolveRead("../escape.txt", wd); err == nil {
		t.Error("expected error for relative ../ escape")
	}
}

func TestResolveRead_RejectsSymlinkEscape(t *testing.T) {
	t.Parallel()
	wd := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.txt")
	mustWriteFile(t, outside, "secret")

	link := filepath.Join(wd, "shortcut.txt")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	if _, err := ResolveRead("shortcut.txt", wd); err == nil {
		t.Error("expected error: symlink should not bypass workdir boundary")
	}
}

func TestResolveRead_RejectsSensitiveLocations(t *testing.T) {
	// no t.Parallel — t.Setenv requires non-parallel tests
	home := t.TempDir()
	t.Setenv("HOME", home)

	wd := filepath.Join(home, "project")
	mustMkdirAll(t, wd)

	sshDir := filepath.Join(home, ".ssh")
	mustMkdirAll(t, sshDir)
	mustWriteFile(t, filepath.Join(sshDir, "id_rsa"), "key")

	if _, err := ResolveRead(filepath.Join(home, ".ssh", "id_rsa"), wd); err == nil {
		t.Error("expected error reading ~/.ssh/id_rsa")
	}
}

func TestResolveRead_RejectsSensitiveBasenameInsideWorkdir(t *testing.T) {
	t.Parallel()
	wd := t.TempDir()
	mustWriteFile(t, filepath.Join(wd, "id_rsa"), "fake")

	if _, err := ResolveRead("id_rsa", wd); err == nil {
		t.Error("expected error reading id_rsa even inside workdir")
	}
}

func TestResolveRead_AllowsEnvExample(t *testing.T) {
	t.Parallel()
	wd := t.TempDir()
	mustWriteFile(t, filepath.Join(wd, ".env.example"), "FOO=bar")

	if _, err := ResolveRead(".env.example", wd); err != nil {
		t.Errorf(".env.example should be allowed, got %v", err)
	}
}

func TestResolveRead_BlocksDotEnv(t *testing.T) {
	t.Parallel()
	wd := t.TempDir()
	mustWriteFile(t, filepath.Join(wd, ".env"), "SECRET=x")
	mustWriteFile(t, filepath.Join(wd, ".env.local"), "SECRET=y")

	if _, err := ResolveRead(".env", wd); err == nil {
		t.Error(".env should be blocked")
	}
	if _, err := ResolveRead(".env.local", wd); err == nil {
		t.Error(".env.local should be blocked")
	}
}

func TestResolveWrite_RejectsOutsideWorkdir(t *testing.T) {
	t.Parallel()
	wd := t.TempDir()
	outside := filepath.Join(t.TempDir(), "evil.txt")

	if _, err := ResolveWrite(outside, wd); err == nil {
		t.Error("expected error writing outside workdir")
	}
}

func TestResolveWrite_AcceptsNewFileInsideWorkdir(t *testing.T) {
	t.Parallel()
	wd := t.TempDir()
	target := filepath.Join(wd, "new", "file.txt")

	got, err := ResolveWrite("new/file.txt", wd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	realWd, _ := filepath.EvalSymlinks(wd)
	wantReal := filepath.Join(realWd, "new", "file.txt")
	if got != wantReal && got != target {
		t.Errorf("got %q, want %q (or %q)", got, wantReal, target)
	}
}

func TestIsSensitive_Patterns(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		"/x/.env":            true,
		"/x/.env.local":      true,
		"/x/.env.example":    false,
		"/x/.env.sample":     false,
		"/x/main.go":         false,
		"/x/foo/id_rsa":      true,
		"/x/foo/server.pem":  true,
		"/x/foo/key.pub":     false,
		"/x/foo/key.pub.txt": false,
		"/x/credentials":     true,
		"/x/foo/.netrc":      true,
	}
	for path, want := range cases {
		got := IsSensitive(path)
		if got != want {
			t.Errorf("IsSensitive(%q) = %v, want %v", path, got, want)
		}
	}
}

// helpers
func mustMkdirAll(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", p, err)
	}
}

func mustWriteFile(t *testing.T, p, content string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
}

// Sanity: the sensitive-path list must not be empty even when HOME is set to
// a temp dir during tests.
func TestSensitiveDirs_NotEmpty(t *testing.T) {
	// no t.Parallel — t.Setenv requires non-parallel tests
	t.Setenv("HOME", t.TempDir())
	dirs := sensitiveDirs()
	if len(dirs) == 0 {
		t.Fatal("sensitiveDirs() returned empty list")
	}
	for _, d := range dirs {
		if !strings.Contains(d, ".") {
			t.Errorf("sensitive dir %q does not look like a hidden dir", d)
		}
	}
}
