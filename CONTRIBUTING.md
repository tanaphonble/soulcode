# Contributing to soulcode

Thank you for your interest. This document covers everything you need to contribute effectively.

---

## Prerequisites

- Go 1.25+
- `git`
- An LLM API key **or** [Ollama](https://ollama.com) for local testing

---

## Setup

```sh
git clone https://github.com/tanaphonble/soulcode
cd soulcode
go build ./...
go test ./...
```

No additional tooling required. soulcode has zero external dependencies.

---

## Project structure

```
soulcode/
├── main.go                         entry point
└── internal/
    ├── config/       config load, save, setup wizard
    ├── context/      project context (git, soulcode.md) → system prompt
    ├── provider/     LLM provider interface and types
    │   ├── anthropic/    Anthropic implementation
    │   └── openai/       OpenAI-compatible implementation
    ├── session/      conversation history
    ├── repl/         interactive REPL and agentic loop
    └── tools/        built-in tools (bash, file ops, search)
```

---

## Adding a provider

1. Create `internal/provider/<name>/client.go` implementing `provider.Provider`:

```go
package myprovider

import (
    "context"
    "soulcode/internal/provider"
)

type Client struct { /* ... */ }

func New(apiKey, model string) (*Client, error) { /* ... */ }
func (c *Client) ID() string { return "myprovider/" + c.model }
func (c *Client) Chat(ctx context.Context, messages []provider.Message, tools []provider.Tool) (<-chan provider.Event, error) {
    // stream SSE, emit Event{Text:...} for text, Event{ToolCall:...} for tool calls
}
```

2. Register it in `main.go` inside `buildProvider`:

```go
case "myprovider":
    return myprovider.New(cfg.APIKey, cfg.Model)
```

3. Add it to the setup wizard in `internal/config/config.go`.

4. Write tests using `httptest.NewServer` — see `internal/provider/anthropic/client_test.go` for the pattern.

---

## Adding a tool

1. Add a file in `internal/tools/` (or a new function in an existing file):

```go
func myTool() (provider.Tool, executeFn) {
    return provider.Tool{
        Name:        "my_tool",
        Description: "Does something useful.",
        Schema: schema(`{
            "type": "object",
            "properties": {
                "input": {"type": "string", "description": "The input."}
            },
            "required": ["input"]
        }`),
    }, func(ctx context.Context, raw json.RawMessage) (string, error) {
        var args struct{ Input string `json:"input"` }
        if err := json.Unmarshal(raw, &args); err != nil {
            return "", fmt.Errorf("my_tool: %w", err)
        }
        // implement
        return result, nil
    }
}
```

2. Register it in `internal/tools/tools.go` inside `New()`:

```go
r.register(myTool())
```

3. Write tests in `internal/tools/mytool_test.go` — see `files_test.go` for the pattern.

---

## Running tests

```sh
# all tests
go test ./...

# specific package
go test ./internal/tools/...

# with race detector
go test -race ./...

# verbose
go test -v ./...
```

All tests are offline — no real API calls are made. Provider tests use `httptest.NewServer`.

---

## Code style

- Follow standard Go conventions (`gofmt`, `go vet`)
- Table-driven tests with `t.Parallel()`
- `t.TempDir()` for any file system operations in tests
- Errors wrapped with `fmt.Errorf("pkg: %w", err)`
- No global state; dependencies injected through constructors
- Comments only when the *why* is non-obvious

---

## Pull request guidelines

- One logical change per PR
- Tests required for new tools and providers
- `go test -race ./...` must pass
- Keep the PR description focused on *why*, not *what* (the diff shows what)

---

## Reporting issues

Open an issue at https://github.com/tanaphonble/soulcode/issues with:
- soulcode version (`soulcode --version`)
- Provider and model
- Steps to reproduce
