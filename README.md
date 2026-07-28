# LoopAI-MCP

Control any TUI-based AI coding agent (Claude Code, OpenCode, etc.) through **terminal I/O** — the same interface a human user would use. No SDKs, no plugins, no vendor lock-in.

## How it works

```
loopai (launcher)                    loopai-backend
  allocates PTY                        reads terminal output
  spawns claude/opencode inside        detects compile/lint/test phases
  streams I/O to backend ◄──socket──►  types prompts, sends Ctrl+C
  detects idle timeout                 enforces: compile → lint → test
```

The launcher replaces your normal CLI — run `loopai` instead of `claude`. The backend is the brain that enforces the hard rule loop.

## Quickstart

```
# Install both binaries
go install github.com/romayengineer/loopai-mcp/cmd/...@latest

# Terminal 1: start the backend
loopai-backend

# Terminal 2: run a task through Claude Code
loopai -client claude "fix the failing tests"
```

The backend watches terminal output. When it detects `go build`/`go test`/`golangci-lint` commands, it analyzes the output. If a step fails, it prompts the model to fix it before proceeding to the next gate.

## Commands

| Command | Description |
|---|---|
| `loopai-backend` | Start the backend (Unix socket at `~/.config/loopai/loopai.sock`) |
| `loopai "prompt"` | Run a one-shot prompt through the default client |
| `loopai -client opencode "prompt"` | Run through a different client |
| `loopai -client echo "hello"` | Test with any binary (echo, sh, etc.) |

### Flags

| Flag | Default | Description |
|---|---|---|
| `-client` | `claude` (or `$LOOPAI_CLIENT`) | Client binary to spawn |
| `-socket` | `~/.config/loopai/loopai.sock` | Unix socket path |
| `-idle` | `5s` | Idle timeout before signaling backend |

## Enforcement gates

The backend enforces: **compile → lint → test**

On each idle event, buffered output is analyzed:
- **compile**: detects `go build`/`go vet` errors (`.go:line:col:` patterns) — empty output = success
- **lint**: detects `golangci-lint` issues (same file:line:col format) — empty output = success
- **test**: detects `ok` (pass) vs `--- FAIL:` (failure) patterns

If a step fails, the backend types a fix prompt into the terminal. The model fixes the code, re-runs the tool, and the loop continues until all gates pass.

Phase detection is language-specific. Currently supports Go tools only. Extensible via configuration.

## Architecture

See `docs/` for detailed architecture docs:
- `docs/claude-code-mcp-client.md` — how Claude Code works as an MCP host
- `docs/opencode-client-architecture.md` — how OpenCode works
- `docs/go-over-rust.md` — why Go for this project

## Testing

```
make test          # unit + integration (excl. smoke)
make test-all      # unit + integration + smoke
make test-unit     # fast tests, no external deps
make test-integration  # real I/O (sockets, PTY)
```

Integration tests use `//go:build integration` tags and exercise real I/O with no mocking.

## License

MIT
