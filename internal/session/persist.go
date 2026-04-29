package session

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tanaphonble/soulcode/internal/provider"
)

type savedSession struct {
	Name     string             `json:"name,omitempty"`
	WorkDir  string             `json:"work_dir,omitempty"`
	SavedAt  time.Time          `json:"saved_at"`
	Messages []provider.Message `json:"messages"`
}

// sessionsDir returns the base directory for all session storage.
func sessionsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".soulcode", "sessions")
}

// autoKey derives a session key from the working directory (hash-based, anonymous).
func autoKey(workDir string) string {
	return "auto-" + fmt.Sprintf("%x", sha256.Sum256([]byte(workDir)))[:12]
}

// sessionPath returns the full path to the session file for a given key.
func sessionPath(key string) string {
	return filepath.Join(sessionsDir(), key, "latest.json")
}

// Save persists the session messages (excluding system prompt) to disk.
func (s *Session) Save(key string) error {
	path := sessionPath(key)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	saved := savedSession{
		SavedAt:  time.Now(),
		Messages: s.messages,
	}
	if !strings.HasPrefix(key, "auto-") {
		saved.Name = key
	}
	data, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// Load restores messages from a previous session if one exists.
// Returns (true, nil) if a session was restored.
func (s *Session) Load(key string) (bool, error) {
	path := sessionPath(key)
	data, err := os.ReadFile(path) //nolint:gosec // path is constructed from trusted key, not raw user input
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var saved savedSession
	if err := json.Unmarshal(data, &saved); err != nil {
		return false, err
	}
	s.messages = saved.Messages
	return true, nil
}

// DeleteSaved removes the persisted session for the given key.
func (s *Session) DeleteSaved(key string) {
	_ = os.Remove(sessionPath(key))
}

// SessionInfo holds display metadata for a saved session.
type SessionInfo struct {
	Key     string
	Name    string
	SavedAt time.Time
	Count   int
}

// ListNamed returns all named (non-auto) saved sessions, newest first.
func ListNamed() ([]SessionInfo, error) {
	base := sessionsDir()
	entries, err := os.ReadDir(base)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var out []SessionInfo
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), "auto-") {
			continue
		}
		p := filepath.Join(base, e.Name(), "latest.json")
		data, err := os.ReadFile(p) //nolint:gosec // path constructed from directory listing
		if err != nil {
			continue
		}
		var saved savedSession
		if err := json.Unmarshal(data, &saved); err != nil {
			continue
		}
		name := saved.Name
		if name == "" {
			name = e.Name()
		}
		out = append(out, SessionInfo{
			Key:     e.Name(),
			Name:    name,
			SavedAt: saved.SavedAt,
			Count:   len(saved.Messages),
		})
	}
	// Sort newest first.
	for i := 0; i < len(out)-1; i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].SavedAt.After(out[i].SavedAt) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out, nil
}

// AutoKey is the public accessor used by the REPL to derive a workdir key.
func AutoKey(workDir string) string { return autoKey(workDir) }
