package session_test

import (
	"strings"
	"testing"

	"github.com/tanaphonble/soulcode/internal/provider"
	"github.com/tanaphonble/soulcode/internal/session"
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

func addTurn(s *session.Session, userMsg, assistantMsg, toolResult string) {
	s.Add(provider.Message{Role: provider.RoleUser, Content: userMsg})
	s.Add(provider.Message{
		Role:      provider.RoleAssistant,
		Content:   assistantMsg,
		ToolCalls: []provider.ToolCall{{ID: "t1", Name: "bash"}},
	})
	s.Add(provider.Message{Role: provider.RoleTool, Content: toolResult, ToolCallID: "t1"})
}

func TestMessagesForAPI_CompressesOldToolResults(t *testing.T) {
	t.Parallel()
	s := session.New("sys")

	longOutput := strings.Repeat("x", 2000)
	addTurn(s, "turn1", "resp1", longOutput)
	addTurn(s, "turn2", "resp2", longOutput)
	addTurn(s, "turn3", "resp3", longOutput) // recent — should stay full

	msgs := s.MessagesForAPI(100, 2)

	var toolMsgs []provider.Message
	for _, m := range msgs {
		if m.Role == provider.RoleTool {
			toolMsgs = append(toolMsgs, m)
		}
	}
	if len(toolMsgs) != 3 {
		t.Fatalf("expected 3 tool messages, got %d", len(toolMsgs))
	}
	if len(toolMsgs[0].Content) >= 2000 {
		t.Errorf("old tool result should be compressed, got len=%d", len(toolMsgs[0].Content))
	}
	if !strings.HasSuffix(toolMsgs[0].Content, "…[truncated]") {
		t.Errorf("expected truncation marker, got %q", toolMsgs[0].Content)
	}
	if len(toolMsgs[2].Content) < 2000 {
		t.Errorf("recent tool result should not be compressed, got len=%d", len(toolMsgs[2].Content))
	}
}

func TestMessagesForAPI_ShortResultsUntouched(t *testing.T) {
	t.Parallel()
	s := session.New("")
	addTurn(s, "q", "a", "short output")
	addTurn(s, "q2", "a2", "also short")

	msgs := s.MessagesForAPI(100, 1)
	for _, m := range msgs {
		if m.Role == provider.RoleTool && strings.Contains(m.Content, "truncated") {
			t.Errorf("short result should not be truncated: %q", m.Content)
		}
	}
}

func TestMessagesForAPI_DoesNotMutateSession(t *testing.T) {
	t.Parallel()
	s := session.New("")
	long := strings.Repeat("y", 2000)
	addTurn(s, "q", "a", long)
	addTurn(s, "q2", "a2", "recent")

	_ = s.MessagesForAPI(100, 1)

	for _, m := range s.Messages() {
		if m.Role == provider.RoleTool && m.Content != "recent" && len(m.Content) < 2000 {
			t.Error("MessagesForAPI must not mutate original session messages")
		}
	}
}
