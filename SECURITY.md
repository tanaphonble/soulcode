# Security Policy

## Reporting a vulnerability

Please **do not** open a public GitHub issue for security vulnerabilities.

Use [GitHub's private security advisory](https://github.com/tanaphonble/soulcode/security/advisories/new) instead. Include:
- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Any suggested fix

You will receive an initial response within 48 hours. A patch is targeted within 7 days of confirmation.

## Supported versions

| Version | Supported |
|---|---|
| latest | ✓ |

---

## Threat model

soulcode is an interactive AI coding assistant. It executes shell commands and edits files on the user's machine, and it transmits portions of the user's source code to a third-party LLM provider. The threats below are addressed by the design; everything else is explicitly **out of scope** for soulcode's defences and must be handled by the user (e.g. by running soulcode in a container).

### What soulcode protects against

| Threat | Mitigation |
|---|---|
| LLM (or prompt-injected file content) instructs reads outside the project | `read_file`, `ls`, `grep`, `glob` resolve paths against the working directory and reject anything outside it; symlinks that escape are rejected after `EvalSymlinks` |
| LLM instructs writes outside the project | `write_file`, `edit_file` apply the same boundary check, including for not-yet-existent paths (parent chain is resolved) |
| LLM reads sensitive files even within the project (`id_rsa`, `.env`, `*.pem`, etc.) | Sensitive basenames, extensions, and known directories (`~/.ssh`, `~/.aws`, `~/.gnupg`, `~/.docker`, `~/.kube`, `~/.soulcode`) are blocked unconditionally |
| LLM hallucinates a destructive bash command | bash requires user approval per command. `--yolo` skips the prompt for routine commands but `rm -rf /`, `curl … \| sh`, `dd of=/dev/sd…`, fork bombs, `git push --force`, and similar patterns always re-prompt |
| Tool output containing a credential is sent to the LLM provider | Tool results are scanned for AWS / GitHub / OpenAI / Anthropic / Stripe / Slack / GPG / JWT / PEM private-key shapes and redacted before being added to the conversation |
| Compromised endpoint via crafted base URL (`http://localhost.evil.com`) | OpenAI-compatible base URL is parsed with `net/url` and the hostname is whitelist-checked against `localhost`, `127.0.0.1`, `::1` for HTTP; everything else must be HTTPS |
| Plaintext API key on disk | Config and session files are written with mode `0600` and live under a `0700` directory. `ANTHROPIC_API_KEY` / `OPENAI_API_KEY` / `SOULCODE_API_KEY` env vars override the on-disk key |
| Forensic gap if the agent does something unexpected | Every tool call is appended to `~/.soulcode/audit.log` (mode `0600`) with the approval label, exit status, and a SHA-256 of the result |

### What soulcode does **not** protect against

These are real risks. soulcode does not pretend to mitigate them.

- **Kernel- or OS-level escapes.** soulcode runs in your user account and has the same filesystem and network reach you do. Path scoping is in user-space and can be defeated by a sufficiently determined attacker who has already convinced you to approve a bash command.
- **Provider-side data retention.** Code that you intentionally show to the model is sent to the provider you configured. soulcode does not encrypt traffic beyond standard TLS and has no view into the provider's logs.
- **Malicious local LLM endpoints.** If you point soulcode at a custom base URL, that endpoint sees everything the model would have seen, plus your tool results. Choose your endpoints carefully.
- **Supply-chain attacks on dependencies.** soulcode's Go module graph is small but not zero. Verify `go.sum` and review changes when updating.
- **The user themselves.** If you type a secret into the prompt, soulcode cannot tell it apart from a normal question. Avoid pasting raw credentials.

### Recommended deployment

For agentic workflows that you do not want to babysit, run soulcode inside an ephemeral container:

```bash
docker run --rm -it \
  -v "$PWD":/workspace -w /workspace \
  -e ANTHROPIC_API_KEY \
  golang:1.26 bash -c "go install ./... && soulcode --yolo"
```

This adds a kernel-enforced filesystem and network boundary that soulcode itself does not provide.

## Scope of the bug bounty (informal)

soulcode does not run a paid bounty programme. We will publicly credit anyone who discloses a real vulnerability through a private advisory, with their consent.

In scope:
- Anything that bypasses the workdir boundary, sensitive-path blocklist, or bash approval prompt
- Anything that exfiltrates credentials to a remote endpoint without user consent
- SSRF, command injection, path traversal, TOCTOU races in tool inputs

Out of scope:
- Reports that require the user to type `--yolo` and then approve a dangerous-pattern prompt
- Reports that require the user to add an attacker's URL as a configured base URL
- Reports requiring a modified soulcode binary
