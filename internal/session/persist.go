package session

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"soulcode/internal/provider"
)

type savedSession struct {
	Messages []provider.Message `json:"messages"`
}

// sessionPath returns the path for persisting the session for a given working directory.
func sessionPath(workDir string) string {
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(workDir)))[:12]
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".soulcode", "sessions", hash, "latest.json")
}

// Save persists the session messages (excluding system prompt) to disk.
func (s *Session) Save(workDir string) error {
	path := sessionPath(workDir)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(savedSession{Messages: s.messages}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// Load restores messages from a previous session if one exists.
// Returns (true, nil) if a session was restored.
func (s *Session) Load(workDir string) (bool, error) {
	path := sessionPath(workDir)
	data, err := os.ReadFile(path) //nolint:gosec // path is constructed from a hash, not user input
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

// DeleteSaved removes the persisted session for a working directory.
func (s *Session) DeleteSaved(workDir string) {
	_ = os.Remove(sessionPath(workDir))
}
