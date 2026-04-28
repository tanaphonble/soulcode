// Package session manages conversation history for a single REPL session.
package session

import "soulcode/internal/provider"

// Session holds the ordered message history for a conversation.
type Session struct {
	system   string
	messages []provider.Message
}

// New creates an empty session. system is the optional system prompt prepended
// to every Chat call.
func New(system string) *Session {
	return &Session{system: system}
}

// Add appends a message to the history.
// Session is not thread-safe; callers must ensure single-goroutine access.
func (s *Session) Add(m provider.Message) {
	s.messages = append(s.messages, m)
}

// Messages returns the full message list, prepending the system prompt if set.
// The returned slice is a copy safe for the caller to hold.
func (s *Session) Messages() []provider.Message {
	cap := len(s.messages)
	if s.system != "" {
		cap++
	}
	out := make([]provider.Message, 0, cap)
	if s.system != "" {
		out = append(out, provider.Message{Role: provider.RoleSystem, Content: s.system})
	}
	return append(out, s.messages...)
}

// Clear resets the conversation history without touching the system prompt.
func (s *Session) Clear() {
	s.messages = s.messages[:0]
}

// Len returns the number of user/assistant turns (excludes system prompt).
func (s *Session) Len() int { return len(s.messages) }
