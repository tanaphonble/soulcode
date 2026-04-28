# soulcode

[![CI](https://github.com/tanaphonble/soulcode/actions/workflows/ci.yml/badge.svg)](https://github.com/tanaphonble/soulcode/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/tanaphonble/soulcode)](https://goreportcard.com/report/github.com/tanaphonble/soulcode)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

A terminal-based AI coding assistant that works with any LLM.

```
> implement a Go HTTP middleware that validates JWT tokens
[bash] go build ./...
Done. Created internal/middleware/jwt.go with full implementation and tests.
```

---

## Why soulcode

| | Claude Code | opencode | **soulcode** |
|---|---|---|---|
| Binary size | ~50 MB (Node) | ~150 MB (Node) | **< 10 MB** |
| Startup | ~500 ms | ~1 s | **< 50 ms** |
| LLM providers | Anthropic only | Multi | **Any** |
| Runtime | Node.js | Node.js | **None** |
| Local models | ✗ | Partial | **✓ Ollama** |
| Code leaves machine | Always | Always | **Optional** |
| Open source | ✗ | ✓ | **✓** |

---

## Install

**Go install**
```sh
go install github.com/tanaphonble/soulcode@latest
```

**Build from source**
```sh
git clone https://github.com/tanaphonble/soulcode
cd soulcode
go build -o soulcode .
```

---

## Quick start

```sh
soulcode
```

On first run, an interactive wizard configures your provider and saves to `~/.soulcode/config.json`.

```
Welcome to soulcode. Let's get you set up.

Provider:
  [1] Anthropic  (Claude)
  [2] OpenAI     (GPT)
  [3] Compatible (Groq · Ollama · Together · Mistral · …)

Choice [1]:
```

### Run with Ollama (fully local, no API key)

```sh
# start Ollama
ollama pull llama3.2

# run soulcode — choose [3], base URL: http://localhost:11434/v1
soulcode
```

### Switch provider any time

Edit `~/.soulcode/config.json`:
```json
{
  "provider": "openai",
  "model": "gpt-4o",
  "api_key": "sk-...",
  "base_url": "https://api.openai.com/v1"
}
```

---

## Usage

```
soulcode  anthropic/claude-sonnet-4-6
Ctrl+C cancels · Ctrl+D exits · /help for commands

> refactor the auth package to use middleware pattern
> add unit tests for internal/api/handler.go
> why is this function O(n²)?
```

### Commands

| Command | Description |
|---|---|
| `/clear` | Clear conversation history |
| `/help` | Show commands |
| `/exit` | Exit (also `Ctrl+D`) |

### Keyboard shortcuts

| Key | Action |
|---|---|
| `Ctrl+C` | Cancel current request |
| `Ctrl+D` | Exit |

---

## Tools

soulcode gives the LLM the following tools to work with your codebase:

| Tool | Description |
|---|---|
| `bash` | Execute shell commands (build, test, lint, git…) |
| `read_file` | Read a file |
| `write_file` | Create or overwrite a file |
| `edit_file` | Replace a unique string in a file |
| `ls` | List a directory |
| `grep` | Search files with regex |
| `glob` | Find files by pattern |

---

## soulcode.md

Create a `soulcode.md` file in your project root to give the LLM persistent context about your project:

```markdown
# myproject

Go REST API using Gin and PostgreSQL.

## Conventions
- All handlers live in internal/api/
- Use `errors.As` for error handling, never panic
- Run `make test` before committing
```

soulcode discovers this file automatically (walks up to the git root) and injects it into every session.

---

## Supported providers

| Provider | `provider` value | Notes |
|---|---|---|
| [Anthropic](https://anthropic.com) | `anthropic` | Claude Opus, Sonnet, Haiku |
| [OpenAI](https://openai.com) | `openai` | GPT-4o, o1, o3 |
| [Groq](https://groq.com) | `openai` | Set `base_url` to `https://api.groq.com/openai/v1` |
| [Together](https://together.ai) | `openai` | Set `base_url` to `https://api.together.xyz/v1` |
| [Mistral](https://mistral.ai) | `openai` | Set `base_url` to `https://api.mistral.ai/v1` |
| [Ollama](https://ollama.com) | `openai` | Set `base_url` to `http://localhost:11434/v1` |
| Any OpenAI-compatible API | `openai` | Set `base_url` accordingly |

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

MIT
