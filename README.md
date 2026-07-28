# LoopAI-MCP

Control any TUI-based AI coding agent (Claude Code, OpenCode, etc.) through **terminal I/O** — the same interface a human user would use. No SDKs, no plugins, no vendor lock-in.

## Development

### Prerequisites

- Go 1.22+
- `golangci-lint` (optional, for `make lint`): `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest`

### Setup

```
git clone https://github.com/romayengineer/loopai-mcp
cd loopai-mcp
make install-hooks   # installs pre-commit hooks (gofmt, vet, lint, unit tests)
make build           # builds all binaries
```

### Available commands

| Command | Description |
|---|---|
| `make build` | Build all binaries |
| `make lint` | Run golangci-lint |
| `make test-unit` | Run unit tests with race detector |
| `make test-integration` | Run integration tests (real I/O) |
| `make test-all` | Run all tests |
| `make cover` | Show test coverage percentages |
| `make fmt` | Check gofmt -s formatting |
| `make vet` | Run go vet |

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
|---|---|---|---|
| `-client` | `claude` (or `$LOOPAI_CLIENT`) | Client binary to spawn |
| `-socket` | `~/.config/loopai/loopai.sock` | Unix socket path |
| `-idle` | `5s` | Idle timeout before signaling backend |
| `-passthrough` | `false` | Show PTY output on terminal alongside backend enforcement |
| `-interactive` | `false` | Interactive mode: stdin → PTY, PTY → terminal (implies `-passthrough`). Automatically disables PTY ECHO to prevent raw escape sequences (e.g. `^[[<35u`) from appearing on the terminal when typing. |

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

See [Available commands](#available-commands) for the full list of make targets. Integration tests use `//go:build integration` tags and exercise real I/O with no mocking.

## License

MIT
