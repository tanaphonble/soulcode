// Package config handles loading, persisting, and interactively setting up
// soulcode's configuration.
package config

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"
)

// Config is the persisted configuration for a soulcode installation.
type Config struct {
	// Provider is the backend identifier: "anthropic" or "openai".
	Provider string `json:"provider"`
	// Model is the model name passed to the provider API.
	Model string `json:"model"`
	// APIKey is the credential for the provider. May be empty for local endpoints.
	APIKey string `json:"api_key,omitempty"`
	// BaseURL overrides the default API endpoint. Required for OpenAI-compatible
	// providers that are not the official OpenAI service.
	BaseURL string `json:"base_url,omitempty"`
}

func path() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".soulcode", "config.json")
}

// Load reads and parses the config file. Returns (nil, nil) when no config
// file exists yet, so callers can distinguish "not configured" from errors.
func Load() (*Config, error) {
	data, err := os.ReadFile(path())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("config: read: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("config: parse: %w", err)
	}
	return &cfg, nil
}

// Save writes cfg to disk, creating parent directories as needed.
// Permissions are set to 0600 to protect credentials.
func Save(cfg *Config) error {
	p := path()
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return fmt.Errorf("config: create directory: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ") //nolint:gosec // intentional: API key is user's own credential
	if err != nil {
		return fmt.Errorf("config: marshal: %w", err)
	}
	if err := os.WriteFile(p, data, 0600); err != nil {
		return fmt.Errorf("config: write: %w", err)
	}
	return nil
}

// Setup runs an interactive wizard that guides the user through first-time
// configuration and saves the result to disk.
func Setup() (*Config, error) {
	r := bufio.NewReader(os.Stdin)

	fmt.Println("Welcome to soulcode. Let's get you set up.")
	fmt.Println()

	providers := []struct {
		label    string
		name     string
		defModel string
		defURL   string
		needsKey bool
	}{
		{"Anthropic  (Claude)", "anthropic", "claude-sonnet-4-6", "", true},
		{"OpenAI     (GPT)", "openai", "gpt-4o", "https://api.openai.com/v1", true},
		{"Compatible (Groq · Ollama · Together · Mistral · …)", "openai", "", "", false},
	}

	fmt.Println("Provider:")
	for i, p := range providers {
		fmt.Printf("  [%d] %s\n", i+1, p.label)
	}
	fmt.Print("\nChoice [1]: ")

	choiceStr := readLine(r)
	if choiceStr == "" {
		choiceStr = "1"
	}

	idx := 0
	switch choiceStr {
	case "1":
		idx = 0
	case "2":
		idx = 1
	case "3":
		idx = 2
	default:
		return nil, fmt.Errorf("config: invalid choice %q", choiceStr)
	}

	sel := providers[idx]
	cfg := &Config{Provider: sel.name, BaseURL: sel.defURL}

	if idx == 2 {
		// Compatible endpoint needs a base URL.
		fmt.Print("Base URL (e.g. http://localhost:11434/v1): ")
		cfg.BaseURL = readLine(r)
		if cfg.BaseURL == "" {
			return nil, fmt.Errorf("config: base URL is required for compatible providers")
		}
	}

	fmt.Printf("Model [%s]: ", sel.defModel)
	model := readLine(r)
	if model == "" {
		model = sel.defModel
	}
	if model == "" {
		return nil, fmt.Errorf("config: model name is required")
	}
	cfg.Model = model

	if sel.needsKey || idx != 2 {
		if idx == 2 {
			fmt.Print("API key (leave blank if not required): ")
		} else {
			fmt.Print("API key: ")
		}
		cfg.APIKey = readSecret()
	}

	if err := Save(cfg); err != nil {
		return nil, err
	}

	fmt.Printf("\nConfiguration saved to ~/.soulcode/config.json\n\n")
	return cfg, nil
}

func readLine(r *bufio.Reader) string {
	s, _ := r.ReadString('\n')
	return strings.TrimSpace(s)
}

// readSecret reads a line from stdin without echoing — used for API keys.
func readSecret() string {
	if term.IsTerminal(int(os.Stdin.Fd())) { //nolint:gosec // standard terminal API: uintptr→int is safe on all supported platforms
		b, err := term.ReadPassword(int(os.Stdin.Fd())) //nolint:gosec
		fmt.Println()                                   // newline after hidden input
		if err == nil {
			return strings.TrimSpace(string(b))
		}
	}
	// Fallback for non-terminal (piped input, CI, etc.)
	r := bufio.NewReader(os.Stdin)
	s, _ := r.ReadString('\n')
	return strings.TrimSpace(s)
}
