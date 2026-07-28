# OpenCode Client Architecture

## Overview

OpenCode is an open-source (MIT) AI coding agent written in **TypeScript**, distributed as a native binary. It follows a **client-server architecture**: when you run `opencode`, it starts an HTTP server and a TUI client that connects to it. The server exposes an OpenAPI 3.1 spec at `http://localhost:4096/doc`.

```
opencode binary
  ├── HTTP Server (localhost:4096)
  │     ├── REST API (sessions, messages, files, config, tools, MCP)
  │     └── SSE events  (/global/event, /session/:id/event)
  │
  ├── TUI client (default)
  ├── opencode web (browser client)
  └── IDE plugins (VS Code, JetBrains) — use the server API
```

## License

**MIT License** — full permissiveness. The repository (anomalyco/opencode) is MIT, as is `@opencode-ai/sdk`. No commercial restrictions.

## Supported Languages

### OpenCode SDK (TypeScript only)

| Package | Language | Purpose |
|---|---|---|
| `@opencode-ai/sdk` (npm) | TypeScript | Type-safe HTTP client for the server API |
| `@opencode-ai/plugin` (npm) | TypeScript | Types for writing plugins |

No Python SDK exists. To interact programmatically from other languages, use the OpenAPI 3.1 spec at `http://localhost:4096/doc` to generate an HTTP client.

### MCP Servers (any language)

MCP servers can be built in any language with an MCP SDK (Python, TypeScript, Java, Kotlin, C# — same as Claude Code).

## Architecture Detail

### Client-Server Model

Unlike Claude Code (which embeds its loop in the CLI process), OpenCode decouples the server from the client:

```
opencode serve (standalone, no TUI)
  └── HTTP server with OpenAPI 3.1 spec

Any HTTP client can connect:
  ├── TUI (built-in, default)
  ├── opencode web
  ├── IDE plugins
  ├── @opencode-ai/sdk
  └── curl / any HTTP library
```

When you run `opencode` without arguments, it starts both a TUI and a server. The TUI is just one client of the server — it talks to the server over HTTP and drives the interface.

### No Agent SDK for Owning the Loop

**Critical difference from Claude Code:** OpenCode does NOT have an SDK equivalent to `claude-agent-sdk` that lets you own the agent loop programmatically. There is no way to import a library, receive message streams, and drive tool execution from your own code.

The available integration surfaces are:

| Surface | What you can do | Limits |
|---|---|---|
| HTTP Server API | Create sessions, send prompts, read messages, list files, query MCP status | You send a prompt, wait for the full response — no streaming control, no loop ownership |
| Plugin hooks | Intercept tool execution (`tool.execute.before`, `tool.execute.after`), subscribe to events (`session.*`, `file.*`, etc.) | Can block/modify tool calls, but **cannot inject prompts** into the conversation. Hooks run inline but return only decisions (block or pass) |
| MCP tools | Expose tools the LLM can call | Pull-based only — Claude decides when to call them |
| SSE event stream | Subscribe to `GET /global/event` for real-time session events | Read-only observation, no control |

### How This Affects LoopAI-MCP

For Claude Code, the shim wraps the Agent SDK and owns the loop — it intercepts every message and decides allow/deny/inject.

For OpenCode, the shim must work differently:

```
                 ┌──────────────────────────────────────┐
                 │         Go Backend (server)           │
                 │  - receives events from shim          │
                 │  - decides: allow / deny / inject     │
                 │  (same as Claude Code flow)           │
                 └──────────┬───────────────────────────┘
                            │ HTTP
                 ┌──────────▼───────────────────────────┐
                 │      Client Shim (TypeScript)         │
                 │  - runs as an OpenCode plugin         │
                 │  - hooks tool.execute.before/after    │
                 │  - forwards events to Go backend      │
                 │  - can block tool calls (deny)        │
                 │  - CANNOT inject prompts natively     │
                 └──────────┬───────────────────────────┘
                            │ plugin hooks
                 ┌──────────▼───────────────────────────┐
                 │          OpenCode Server              │
                 │  - manages the agent loop             │
                 │  - executes tools                     │
                 │  - owns conversation state            │
                 └──────────────────────────────────────┘
```

**The shim cannot own the loop.** OpenCode owns its own loop. The shim hooks into it at specific points but cannot take over message flow.

### Prompt Injection Workaround

Since the plugin hook system cannot inject prompts into the conversation, the shim uses a workaround. When the Go backend returns `inject`:

1. The shim calls the OpenCode HTTP API `POST /session/:id/message` to send the injected prompt as a new user message
2. The LLM receives it and responds
3. The plugin hook captures subsequent tool calls

This means the shim is both a **plugin** (for tool event interception) and an **HTTP API client** (for prompt injection). It is a hybrid approach.

### Two communication channels (same as Claude Code)

```
Two independent communication channels:

1. Event stream (shim ↔ backend):
   HTTP/WS, carries all events and decisions
   (tool_execute, tool_result, session events, etc.)

2. MCP protocol (OpenCode LLM ↔ backend):
   Standard MCP, carries tool invocations only
   (compile, lint, test tools)
```

## Events Generated by OpenCode

OpenCode emits events through two mechanisms: the **SSE event stream** (`GET /global/event`) and **plugin hooks**. These are the events a Go backend would receive from the shim.

### SSE Stream Events (via HTTP)

Realtime events available at `GET /global/event`:

| Event | When | Notes |
|---|---|---|
| `server.connected` | Client connects to SSE stream | First event on connect |
| `session.created` | A new session is created | |
| `session.updated` | Session properties change | |
| `session.deleted` | A session is deleted | |
| `session.status` | Session status changes | Includes running/idle states |
| `session.idle` | Agent goes idle (waiting for user) | |
| `session.error` | Session encounters an error | |
| `session.diff` | Files changed in session | |
| `session.compacted` | Context compaction occurred | |

### Plugin Hook Events

Plugins intercept events through named hook functions:

#### Tool Events

| Hook | When | Can block? | Can inject? |
|---|---|---|---|
| `tool.execute.before` | Before a tool executes | Yes (throw error) | No — only block/pass |
| `tool.execute.after` | After a tool succeeds | No | No — read-only observation |

The `tool.execute.before` hook receives `input.tool` (tool name) and `output.args`. Throw an error to block execution. Return normally to allow it.

#### Session Events

| Hook | When |
|---|---|
| `session.created` | Session created |
| `session.deleted` | Session deleted |
| `session.updated` | Session properties changed |
| `session.compacted` | Context was compacted |
| `session.error` | Error in session |

#### File Events

| Hook | When |
|---|---|
| `file.edited` | File was edited |
| `file.watcher.updated` | Watched file changed |

#### Permission Events

| Hook | When |
|---|---|
| `permission.asked` | Permission prompt shown to user |
| `permission.replied` | User responded to permission |

#### Message Events

| Hook | When |
|---|---|
| `message.part.updated` | Part of a message updated |
| `message.part.removed` | Part removed from message |
| `message.updated` | Message updated |
| `message.removed` | Message removed |

#### Other Events

| Hook | When |
|---|---|
| `command.executed` | Slash command executed |
| `shell.env` | Shell environment variables set |
| `lsp.client.diagnostics` | LSP diagnostics updated |
| `lsp.updated` | LSP server state changed |
| `installation.updated` | Plugin/package installed |
| `todo.updated` | Todo list changes |
| `tui.prompt.append` | Text appended to prompt |
| `tui.command.execute` | Command executed from TUI |
| `tui.toast.show` | Toast notification shown |
| `server.connected` | New client connected to server |

## Built-in Tools

| Tool | Description | Permission key |
|---|---|---|
| `bash` | Execute shell commands | `bash` |
| `edit` | Modify files via exact string replacement | `edit` |
| `write` | Create or overwrite files | `edit` (same) |
| `apply_patch` | Apply patches/diffs | `edit` (same) |
| `read` | Read file contents | `read` |
| `grep` | Search file contents with regex | `grep` |
| `glob` | Find files by pattern | `glob` |
| `lsp` | Code intelligence (definitions, references, etc.) | `lsp` |
| `skill` | Load a SKILL.md file | `skill` |
| `todowrite` | Manage task lists during session | `todowrite` |
| `webfetch` | Fetch web page content | `webfetch` |
| `websearch` | Search the web (Exa AI) | `websearch` |
| `question` | Ask the user questions | `question` |

Permission defaults: all `allow` except `.env` files (denied), `external_directory` (ask), `doom_loop` (ask).

## MCP Integration

OpenCode supports MCP servers configured in `opencode.json`:

**Local (stdio):**
```json
{
  "mcp": {
    "my-server": {
      "type": "local",
      "command": ["npx", "-y", "@modelcontextprotocol/server-everything"],
      "enabled": true,
      "environment": { "KEY": "val" }
    }
  }
}
```

**Remote (HTTP):**
```json
{
  "mcp": {
    "my-server": {
      "type": "remote",
      "url": "https://mcp.example.com/mcp",
      "headers": { "Authorization": "Bearer TOKEN" }
    }
  }
}
```

MCP tools are prefixed with the server name and registered alongside built-in tools. They can be enabled/disabled per-agent using `tools` config with glob patterns (e.g., `"mymcp_*": false`).

## Agents

OpenCode has two types of agents:

### Primary Agents

Switchable via **Tab** key. Built-in: **Build** (all tools, default) and **Plan** (read-only, for analysis).

### Subagents

Invoked by primary agents or via `@mention`. Built-in: **General** (full tools), **Explore** (read-only, fast), **Scout** (reads external docs/dependency source).

Custom agents can be defined in `opencode.json` or as Markdown files.

## Deployment

### OpenCode is installed as a native binary

```
curl -fsSL https://opencode.ai/install | bash
npm install -g opencode-ai
brew install anomalyco/tap/opencode
```

### LoopAI-MCP shim runs as a plugin (+ HTTP API client)

The shim is installed as an OpenCode plugin. It also uses the HTTP API for prompt injection.

**Plugin install:**
1. Place JS/TS file in `~/.config/opencode/plugins/` or `.opencode/plugins/`
2. Or publish as npm package and add to `opencode.json` `plugin` array

**Backend is separate** (same as Claude Code approach):
```
go install github.com/romayengineer/loopai-mcp/backend/go/cmd/...@latest
loopai-backend
```

### Shim discovers backend the same way

Env var, CLI flag, or config file.

### Workflow

```
terminal 1 $ loopai-backend
             ← Go backend listening on :8090

terminal 2 $ opencode
             ← plugin hooks into tool.execute.before/after
             ← plugin connects to :8090
             ← forwards events, receives decisions
             ← can block tool calls (deny)
             ← uses HTTP API to inject prompts
```

## Key Differences from Claude Code Integration

| Aspect | Claude Code | OpenCode |
|---|---|---|
| SDK | `claude-agent-sdk` (Python + TS) — own the loop | `@opencode-ai/sdk` (TS only) — HTTP client only |
| Loop control | Full — shim owns the Agent SDK loop | Limited — OpenCode owns its loop, shim hooks in |
| Prompt injection | `agent_sdk.send_message(prompt)` — native | `POST /session/:id/message` — via HTTP API |
| Tool interception | SDK message stream — every tool call | Plugin `tool.execute.before` — limited to block/pass |
| Deny mechanism | Return denial result to LLM via SDK | Throw error in plugin hook to block |
| Client languages | Python or TypeScript | TypeScript only (plugin) + any language (HTTP API) |
| License | MIT + Commercial Terms | MIT (fully open) |
