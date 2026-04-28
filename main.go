package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"soulcode/internal/config"
	"soulcode/internal/provider"
	"soulcode/internal/provider/anthropic"
	"soulcode/internal/provider/openai"
	"soulcode/internal/repl"
)

// Injected at build time via -ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
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
	return repl.New(p, switchFn).Run(ctx)
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
  soulcode            start interactive session
  soulcode version    print version information
  soulcode help       print this message

Configuration:
  ~/.soulcode/config.json   provider, model, and API key

Project context:
  soulcode.md               project instructions (auto-discovered from git root)

More: https://github.com/tanaphonble/soulcode
`
