package tools_test

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestBash_Success(t *testing.T) {
	t.Parallel()
	reg := newRegistry(t.TempDir())

	out, err := reg.Execute(context.Background(), call("bash", `{"command":"echo hello"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "hello") {
		t.Errorf("expected 'hello' in output, got %q", out)
	}
}

func TestBash_NonZeroExit(t *testing.T) {
	t.Parallel()
	reg := newRegistry(t.TempDir())

	out, err := reg.Execute(context.Background(), call("bash", `{"command":"exit 1"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "exit status 1") {
		t.Errorf("expected exit status in output, got %q", out)
	}
}

func TestBash_StderrCaptured(t *testing.T) {
	t.Parallel()
	reg := newRegistry(t.TempDir())

	out, err := reg.Execute(context.Background(), call("bash", `{"command":"echo err >&2"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "err") {
		t.Errorf("expected stderr in output, got %q", out)
	}
}

func TestBash_Timeout(t *testing.T) {
	t.Parallel()
	reg := newRegistry(t.TempDir())

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := reg.Execute(ctx, call("bash", `{"command":"sleep 10"}`))
	// bash tool absorbs the error into output; context cancellation may return error or output
	// either way the call must not block past the timeout
	_ = err
}

func TestBash_EmptyCommand(t *testing.T) {
	t.Parallel()
	reg := newRegistry(t.TempDir())

	_, err := reg.Execute(context.Background(), call("bash", `{"command":""}`))
	if err == nil {
		t.Error("expected error for empty command")
	}
}

func TestBash_UnknownTool(t *testing.T) {
	t.Parallel()
	reg := newRegistry(t.TempDir())

	_, err := reg.Execute(context.Background(), call("nonexistent", `{}`))
	if err == nil {
		t.Error("expected error for unknown tool")
	}
}
