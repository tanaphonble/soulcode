package session_test

import (
	"testing"

	"soulcode/internal/provider"
	"soulcode/internal/session"
)

func TestNew_EmptySession(t *testing.T) {
	t.Parallel()
	s := session.New("")
	if s.Len() != 0 {
		t.Fatalf("expected 0 messages, got %d", s.Len())
	}
}

func TestMessages_WithoutSystemPrompt(t *testing.T) {
	t.Parallel()
	s := session.New("")
	s.Add(provider.Message{Role: provider.RoleUser, Content: "hello"})
	s.Add(provider.Message{Role: provider.RoleAssistant, Content: "world"})

	msgs := s.Messages()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != provider.RoleUser {
		t.Errorf("expected first role user, got %s", msgs[0].Role)
	}
}

func TestMessages_WithSystemPrompt(t *testing.T) {
	t.Parallel()
	s := session.New("you are helpful")
	s.Add(provider.Message{Role: provider.RoleUser, Content: "hello"})

	msgs := s.Messages()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages (system + user), got %d", len(msgs))
	}
	if msgs[0].Role != provider.RoleSystem {
		t.Errorf("expected first role system, got %s", msgs[0].Role)
	}
	if msgs[0].Content != "you are helpful" {
		t.Errorf("unexpected system content: %q", msgs[0].Content)
	}
	if msgs[1].Role != provider.RoleUser {
		t.Errorf("expected second role user, got %s", msgs[1].Role)
	}
}

func TestMessages_ReturnsCopy(t *testing.T) {
	t.Parallel()
	s := session.New("")
	s.Add(provider.Message{Role: provider.RoleUser, Content: "a"})

	msgs := s.Messages()
	msgs[0].Content = "mutated"

	original := s.Messages()
	if original[0].Content == "mutated" {
		t.Error("Messages() returned a slice that shares backing array with internal state")
	}
}

func TestClear(t *testing.T) {
	t.Parallel()
	s := session.New("system")
	s.Add(provider.Message{Role: provider.RoleUser, Content: "hello"})
	s.Add(provider.Message{Role: provider.RoleAssistant, Content: "world"})

	s.Clear()

	if s.Len() != 0 {
		t.Fatalf("expected 0 messages after clear, got %d", s.Len())
	}
	// system prompt must survive clear
	msgs := s.Messages()
	if len(msgs) != 1 || msgs[0].Role != provider.RoleSystem {
		t.Error("system prompt was lost after Clear()")
	}
}

func TestLen_ExcludesSystemPrompt(t *testing.T) {
	t.Parallel()
	s := session.New("system")
	if s.Len() != 0 {
		t.Errorf("Len() should be 0 before any user/assistant messages, got %d", s.Len())
	}
	s.Add(provider.Message{Role: provider.RoleUser, Content: "x"})
	if s.Len() != 1 {
		t.Errorf("expected Len() == 1, got %d", s.Len())
	}
}
