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
	c := len(s.messages)
	if s.system != "" {
		c++
	}
	out := make([]provider.Message, 0, c)
	if s.system != "" {
		out = append(out, provider.Message{Role: provider.RoleSystem, Content: s.system})
	}
	return append(out, s.messages...)
}

// MessagesForAPI returns the message list with old tool results compressed to
// save tokens. Tool results beyond the last keepTurns assistant replies are
// truncated to maxToolLen characters. Recent results are kept intact.
func (s *Session) MessagesForAPI(maxToolLen, keepTurns int) []provider.Message {
	msgs := s.Messages()

	// Find index of the keepTurns-th most recent assistant message.
	cutoff := 0
	seen := 0
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == provider.RoleAssistant {
			seen++
			if seen >= keepTurns {
				cutoff = i
				break
			}
		}
	}

	out := make([]provider.Message, len(msgs))
	copy(out, msgs)
	for i := 0; i < cutoff; i++ {
		if out[i].Role == provider.RoleTool && len(out[i].Content) > maxToolLen {
			out[i].Content = out[i].Content[:maxToolLen] + "…[truncated]"
		}
	}
	return out
}

// Clear resets the conversation history without touching the system prompt.
func (s *Session) Clear() {
	s.messages = s.messages[:0]
}

// Len returns the number of user/assistant turns (excludes system prompt).
func (s *Session) Len() int { return len(s.messages) }
