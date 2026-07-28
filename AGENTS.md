# LoopAI-MCP

## Architecture

### Terminal I/O Control (not SDK)

LoopAI-MCP controls any TUI-based agent client through its **terminal I/O** — the same interface a human user would use. No client SDK is imported, no plugin system is targeted, no vendor lock-in exists.

- **Launcher** (`launcher/`) allocates a PTY, spawns any TUI agent client (Claude Code, OpenCode, etc.) inside it, streams terminal output to the Go backend, and writes keystrokes received from the backend into the PTY. The client does not know it is being driven programmatically.
- **Go backend** (`backend/go/`) is the brain — reads the terminal output byte stream, detects idle via output timeout, decides when to type prompts or send control sequences (Ctrl+C, etc.), and drives the enforcement loop entirely through I/O.
- **Client-agnostic.** One launcher works with every terminal-based AI coding agent. No Python shim vs TypeScript shim — the launcher is a single binary that talks PTY.

### Why PTY I/O over client SDKs

| Criteria | PTY I/O (this approach) | Per-client SDK |
|---|---|---|
| Client support | Any TUI agent: Claude Code, OpenCode, Continue, Aider, any future tool | One implementation per client, each using a different SDK or plugin API |
| Launcher code | One binary, same language as backend (Go) | N implementations in N languages (Python, TS, etc.) |
| Version coupling | None — the client can update freely | Every SDK version change may break the shim |
| Loop control | Absolute — launcher sees every byte, sends every keystroke | SDK-dependent, limited by what the SDK exposes |
| Prompt injection | Type the prompt into the terminal — identical to a user | SDK-specific APIs, may not exist for all clients |
| Deny/interrupt | Send Ctrl+C — the client handles it natively | Requires SDK support for aborting tool calls |
| Observability | See everything the user would see in the terminal | Limited to what the SDK's event stream exposes |
| Long-term maintenance | Client changes don't affect the launcher | Each client update may require shim changes |

### Hard rule enforcement loop

The server enforces this sequence on every change:

```
compile → lint → test
```

If any step fails, the server prompts the model to fix it before proceeding. The same gates must run in **pre-commit hooks** and **CI** — always add enforcement in all three places.

| Layer | Responsibility |
|---|---|
| Server logic | Reads terminal output, types prompts, drives enforcement |
| Pre-commit | Catches issues locally before commit |
| CI | Final gate, mirrors server enforcement |

## Package boundaries

| Directory | Language | Role |
|---|---|---|
| `cmd/loopai-backend/` | Go | Backend entrypoint — Unix socket listener, dispatches to loop |
| `cmd/loopai/` | Go | Launcher entrypoint — PTY spawn, CLI parsing, socket connect |
| `internal/backend/` | Go | Backend server + enforcement loop state machine |
| `internal/launcher/` | Go | PTY lifecycle, I/O streaming, idle detection |
| `internal/proto/` | Go | Shared message types, Unix socket connect/listen helpers |

## Commands

Root-level `make` (or `task`) orchestrates all sub-projects. Fill in exact commands as tooling is chosen.

- **Build all:** `go build ./cmd/...`
- **Test all:** `go test ./internal/...`
- **Test single pkg:** `go test ./internal/launcher/`
- **Test integration:** `go test -tags=integration ./internal/...` (runs real I/O, no mocks)
- **Backend:** `loopai-backend` (starts on Unix socket at `~/.config/loopai/loopai.sock`)
- **Launcher:** `loopai -client claude "prompt"` (spawns client, streams I/O to backend)

## Conventions

- Always run the full enforcement gate (**compile → lint → test**) before considering a task done. The server will re-prompt if you skip it.
- When adding new enforcement rules, add them in all three places: server logic, pre-commit hook, CI workflow.
- The launcher must never import a client SDK or plugin package. It speaks PTY I/O only.
- Idle detection is timeout-based (N ms with no output). No client-specific prompt string parsing.
- The Go backend and launcher communicate over a Unix socket using newline-delimited JSON. Protocol types live in `internal/proto/`.
- **Write tests for every change.** Two kinds:
  - **Unit tests** — test a single function/struct in isolation. Fast, no external deps. Stored in `*_test.go` alongside the code.
  - **Integration tests** — test real I/O (sockets, PTY, process spawning). Do NOT mock the OS. Stored in `*_integration_test.go` with `//go:build integration` build tag. Run with `go test -tags=integration`.
- **Every new feature or bugfix must include both unit and integration tests.** The integration test validates the real end-to-end path; the unit test validates edge cases around it.

## Open items

- Exact terminal output state machine design
- PTY library choice for Go
- Timeout value and idle detection strategy
- CI provider and workflow shape
