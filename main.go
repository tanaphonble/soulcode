package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/tanaphonble/soulcode/internal/config"
	"github.com/tanaphonble/soulcode/internal/provider"
	"github.com/tanaphonble/soulcode/internal/provider/anthropic"
	"github.com/tanaphonble/soulcode/internal/provider/openai"
	"github.com/tanaphonble/soulcode/internal/repl"
)

// Injected at build time via -ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	// Handle positional commands before flag parsing.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version", "--version", "-v":
			fmt.Printf("soulcode %s (commit %s, built %s)\n", version, commit, date)
			return
		case "help", "--help", "-h":
			fmt.Print(usage)
			return
		}
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	fs := flag.NewFlagSet("soulcode", flag.ContinueOnError)
	sessionName := fs.String("s", "", "named session to start or resume")
	yolo := fs.Bool("yolo", false, "auto-approve bash commands except dangerous patterns (CI / sandboxed use)")
	fs.Usage = func() { fmt.Print(usage) }
	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg == nil {
		cfg, err = config.Setup()
		if err != nil {
			return err
		}
	}
	applyEnvOverrides(cfg)

	p, err := buildProvider(cfg)
	if err != nil {
		return err
	}

	// SIGTERM triggers a clean exit; SIGINT is handled inside the REPL.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer stop()

	switchFn := func(model string) (provider.Provider, error) {
		cfg.Model = model
		return buildProvider(cfg)
	}
	return repl.New(p, switchFn, repl.Options{
		SessionName: *sessionName,
		Yolo:        *yolo,
	}).Run(ctx)
}

// applyEnvOverrides lets users keep credentials out of ~/.soulcode/config.json
// by setting one of the well-known environment variables. The order of
// precedence is provider-specific then generic, so a misconfigured generic
// var doesn't override a correct one.
func applyEnvOverrides(cfg *config.Config) {
	if v := os.Getenv("SOULCODE_API_KEY"); v != "" {
		cfg.APIKey = v
	}
	switch cfg.Provider {
	case "anthropic":
		if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" {
			cfg.APIKey = v
		}
	case "openai":
		if v := os.Getenv("OPENAI_API_KEY"); v != "" {
			cfg.APIKey = v
		}
	}
}

func buildProvider(cfg *config.Config) (provider.Provider, error) {
	switch cfg.Provider {
	case "anthropic":
		return anthropic.New(cfg.APIKey, cfg.Model)
	case "openai":
		return openai.New(cfg.APIKey, cfg.Model, cfg.BaseURL)
	default:
		return nil, fmt.Errorf("unknown provider %q — edit ~/.soulcode/config.json", cfg.Provider)
	}
}

const usage = `soulcode — AI coding assistant for the terminal

Usage:
  soulcode               start interactive session (auto-session per directory)
  soulcode -s <name>     start or resume a named session
  soulcode --yolo        auto-approve bash (still blocks dangerous patterns)
  soulcode version       print version information
  soulcode help          print this message

Flags:
  -s <name>    named session (persists across directories)
  --yolo       skip bash approval prompts (CI / sandboxed use only)

Environment:
  ANTHROPIC_API_KEY / OPENAI_API_KEY / SOULCODE_API_KEY
       override the API key from config.json (preferred for ephemeral hosts)
  SOULCODE_YOLO=1
       same as --yolo

Configuration:
  ~/.soulcode/config.json   provider, model, and (optional) API key (mode 0600)
  ~/.soulcode/audit.log     append-only log of every tool call (mode 0600)
  .soulcode/allow.txt       per-project bash command prefixes that auto-approve

Project context:
  soulcode.md               project instructions (auto-discovered from git root)

Security:
  - File tools are scoped to the working directory; symlinks that escape are
    rejected.
  - Sensitive files (~/.ssh, ~/.aws, id_rsa, .env, *.pem, etc.) are blocked.
  - bash commands prompt for approval unless --yolo / SOULCODE_YOLO is set.
  - Dangerous patterns (rm -rf /, curl|sh, ...) always re-prompt.
  - Tool output is scanned for secret-shaped strings and redacted before being
    sent to the LLM provider.

More: https://github.com/tanaphonble/soulcode
`
