package ui

import (
	"strings"
	"testing"
)

func TestShortenPath(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, workDir, home, want string
	}{
		{"/Users/ble/Works/proj/internal/foo.go", "/Users/ble/Works/proj", "/Users/ble", "internal/foo.go"},
		{"/Users/ble/.ssh/config", "/Users/ble/Works/proj", "/Users/ble", "~/.ssh/config"},
		{"/etc/passwd", "/Users/ble/Works/proj", "/Users/ble", "/etc/passwd"},
		{"relative/path.go", "/Users/ble/Works/proj", "/Users/ble", "relative/path.go"},
		{"/Users/ble/Works/proj", "/Users/ble/Works/proj", "/Users/ble", "."}, // exact workdir → "."
	}
	for _, tc := range cases {
		got := ShortenPath(tc.in, tc.workDir, tc.home)
		if got != tc.want {
			t.Errorf("ShortenPath(%q, %q, %q) = %q, want %q", tc.in, tc.workDir, tc.home, got, tc.want)
		}
	}
}

func TestShortenPaths_RewritesEmbeddedPaths(t *testing.T) {
	t.Parallel()
	in := "edited /Users/ble/Works/proj/foo.go and /Users/ble/Works/proj/bar.go"
	out := ShortenPaths(in, "/Users/ble/Works/proj", "/Users/ble")
	if !strings.Contains(out, "foo.go") || !strings.Contains(out, "bar.go") {
		t.Errorf("expected both paths rewritten, got %q", out)
	}
	if strings.Contains(out, "/Users/ble/Works/proj/") {
		t.Errorf("absolute path leaked through: %q", out)
	}
}

func TestCollapseLines(t *testing.T) {
	t.Parallel()
	in := "a\nb\nc\nd\ne\nf\ng"
	out := CollapseLines(in, 5)
	lines := strings.Split(out, "\n")
	if len(lines) != 5 {
		t.Errorf("expected 5 lines, got %d: %q", len(lines), out)
	}
	if !strings.Contains(out, "more lines") {
		t.Errorf("expected truncation marker, got %q", out)
	}
}

func TestCollapseLines_ShortInputUntouched(t *testing.T) {
	t.Parallel()
	in := "one\ntwo"
	if got := CollapseLines(in, 5); got != in {
		t.Errorf("short input mutated: %q", got)
	}
}

func TestColourDiff(t *testing.T) {
	t.Parallel()
	in := "- old\n+ new\nunchanged"
	out := ColourDiff(in)
	if !strings.Contains(out, Red+"- old"+Reset) {
		t.Errorf("expected red on minus line: %q", out)
	}
	if !strings.Contains(out, Green+"+ new"+Reset) {
		t.Errorf("expected green on plus line: %q", out)
	}
	if !strings.Contains(out, "unchanged") {
		t.Errorf("expected unchanged line preserved: %q", out)
	}
}

func TestTruncateOneLine(t *testing.T) {
	t.Parallel()
	if got := TruncateOneLine("short", 50); got != "short" {
		t.Errorf("short string mutated: %q", got)
	}
	if got := TruncateOneLine("line one\nline two", 50); strings.Contains(got, "\n") {
		t.Errorf("newline survived: %q", got)
	}
	if got := TruncateOneLine(strings.Repeat("a", 200), 10); len(got) > 14 {
		t.Errorf("truncation too loose: len=%d %q", len(got), got)
	}
}

func TestSummarize_Bash(t *testing.T) {
	t.Parallel()
	got := summarize("bash", []byte(`{"command":"go test ./..."}`), "/x", "/h")
	if got != "go test ./..." {
		t.Errorf("got %q", got)
	}
}

func TestSummarize_Path(t *testing.T) {
	t.Parallel()
	got := summarize("read_file", []byte(`{"path":"/x/internal/foo.go"}`), "/x", "/h")
	if got != "internal/foo.go" {
		t.Errorf("got %q", got)
	}
}

func TestSummarize_WriteIncludesByteCount(t *testing.T) {
	t.Parallel()
	got := summarize("write_file", []byte(`{"path":"/x/foo.go","content":"hello"}`), "/x", "/h")
	if !strings.Contains(got, "5 bytes") || !strings.Contains(got, "foo.go") {
		t.Errorf("got %q", got)
	}
}
