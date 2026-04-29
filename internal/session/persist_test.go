package session_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"soulcode/internal/provider"
	"soulcode/internal/session"
)

func TestSaveLoad_RoundTrip(t *testing.T) {
	t.Parallel()
	key := session.AutoKey(t.TempDir())
	t.Cleanup(func() { _ = os.Remove(sessionFilePath(t, key)) })

	s := session.New("system prompt")
	s.Add(provider.Message{Role: provider.RoleUser, Content: "hello"})
	s.Add(provider.Message{Role: provider.RoleAssistant, Content: "world"})

	if err := s.Save(key); err != nil {
		t.Fatalf("Save: %v", err)
	}

	s2 := session.New("system prompt")
	restored, err := s2.Load(key)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !restored {
		t.Fatal("Load returned false, expected true")
	}
	if s2.Len() != 2 {
		t.Fatalf("expected 2 messages, got %d", s2.Len())
	}
	msgs := s2.Messages()
	if msgs[1].Content != "hello" {
		t.Errorf("unexpected first message: %q", msgs[1].Content)
	}
	if msgs[2].Content != "world" {
		t.Errorf("unexpected second message: %q", msgs[2].Content)
	}
}

func TestLoad_NonExistent_ReturnsFalse(t *testing.T) {
	t.Parallel()
	key := session.AutoKey(t.TempDir())

	s := session.New("")
	restored, err := s.Load(key)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if restored {
		t.Error("expected restored=false for nonexistent session")
	}
	if s.Len() != 0 {
		t.Errorf("expected 0 messages, got %d", s.Len())
	}
}

func TestLoad_CorruptedJSON_ReturnsError(t *testing.T) {
	t.Parallel()
	key := session.AutoKey(t.TempDir())

	s := session.New("")
	s.Add(provider.Message{Role: provider.RoleUser, Content: "x"})
	if err := s.Save(key); err != nil {
		t.Fatalf("Save: %v", err)
	}

	sessionFile := sessionFilePath(t, key)
	t.Cleanup(func() { _ = os.Remove(sessionFile) })

	if err := os.WriteFile(sessionFile, []byte("not valid json"), 0600); err != nil {
		t.Fatalf("corrupt: %v", err)
	}

	s2 := session.New("")
	_, err := s2.Load(key)
	if err == nil {
		t.Error("expected error for corrupted session file")
	}
}

func TestDeleteSaved_RemovesFile(t *testing.T) {
	t.Parallel()
	key := session.AutoKey(t.TempDir())
	t.Cleanup(func() { _ = os.Remove(sessionFilePath(t, key)) })

	s := session.New("")
	s.Add(provider.Message{Role: provider.RoleUser, Content: "x"})
	if err := s.Save(key); err != nil {
		t.Fatalf("Save: %v", err)
	}

	s.DeleteSaved(key)

	s2 := session.New("")
	restored, err := s2.Load(key)
	if err != nil {
		t.Fatalf("unexpected error after delete: %v", err)
	}
	if restored {
		t.Error("expected restored=false after DeleteSaved")
	}
}

func TestSave_SystemPromptNotPersisted(t *testing.T) {
	t.Parallel()
	key := session.AutoKey(t.TempDir())

	s := session.New("secret system prompt")
	s.Add(provider.Message{Role: provider.RoleUser, Content: "hi"})
	if err := s.Save(key); err != nil {
		t.Fatalf("Save: %v", err)
	}

	sessionFile := sessionFilePath(t, key)
	t.Cleanup(func() { _ = os.Remove(sessionFile) })

	data, err := os.ReadFile(sessionFile) //nolint:gosec
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var saved struct {
		Messages []provider.Message `json:"messages"`
	}
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, m := range saved.Messages {
		if m.Role == provider.RoleSystem {
			t.Error("system prompt must not be persisted in session file")
		}
	}
}

// sessionFilePath returns the path for a given session key, mirroring the internal calculation.
func sessionFilePath(t *testing.T, key string) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	return filepath.Join(home, ".soulcode", "sessions", key, "latest.json")
}
