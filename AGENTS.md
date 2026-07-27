# LoopAI-MCP

## Architecture

- **Client shim** (`client/python/`, `client/typescript/`) sends all events to the Go backend: file edits, file reads, command execution, processing, idle, stop states.
- **Go server** (`backend/go/`) is the brain — receives events, decides whether to inject a prompt, let execution continue, or take other actions. Controls the agent lifecycle.
- **Client-agnostic via MCP.** The protocol layer is abstract; both Claude Code and OpenCode use the same event schema. Adding a new client means writing a new shim in the best language for it.

## Hard rule enforcement loop

The server enforces this sequence on every change:

```
compile → lint → test
```

If any step fails, the server prompts the model to fix it before proceeding. The same gates must run in **pre-commit hooks** and **CI** — always add enforcement in all three places.

| Layer | Responsibility |
|---|---|
| Server logic | Prompt-based enforcement, decides next action |
| Pre-commit | Catches issues locally before commit |
| CI | Final gate, mirrors server enforcement |

## Package boundaries

| Directory | Language | Role |
|---|---|---|
| `backend/go/` | Go | Core server — event processing, prompt injection, enforcement |
| `client/python/` | Python | Python MCP shim |
| `client/typescript/` | TypeScript | TypeScript/JS MCP shim |

## Commands

Root-level `make` (or `task`) orchestrates all sub-projects. As tooling is chosen, fill in exact commands per package below.

- **Backend:** `go build ./backend/go/...`, `go test ./backend/go/...`
- **Python:** `[ruff/pytest/poetry/uv — TBD]`
- **TypeScript:** `[eslint/vitest/npm/pnpm — TBD]`

## Conventions

- Always run the full enforcement gate (**compile → lint → test**) before considering a task done. The server will re-prompt if you skip it.
- When adding new enforcement rules, add them in all three places: server logic, pre-commit hook, CI workflow.
- MCP protocol definitions live in a shared location referenced by both client shims and the server.

## Open items

- Exact Go/TS/Python toolchain choices
- Pre-commit framework
- CI provider and workflow shape
- Event schema / protocol spec
