# Claude Code MCP Client Architecture

## Overview

Claude Code acts as an **MCP host** — it instantiates one **MCP client** per configured MCP server, with each client maintaining a dedicated connection. The MCP server exposes tools, resources, and prompts. Claude Code's agent loop decides when to call those server tools as part of its autonomous task execution.

```
Claude Code (MCP Host)
  ├── MCP Client 1 ──── MCP Server A (stdio / http)
  ├── MCP Client 2 ──── MCP Server B
  └── MCP Client 3 ──── MCP Server C
```

## Supported Languages

### Claude Agent SDK (programmatic agent loop)

Two official SDKs for embedding Claude Code's agent loop into your own application:

| Language | Package | License | Min Version |
|---|---|---|---|
| **Python** | `claude-agent-sdk` (PyPI) | MIT | Python 3.10+ |
| **TypeScript** | `@anthropic-ai/claude-agent-sdk` (npm) | MIT | Node.js 18+ |

These are the SDKs used by the LoopAI-MCP client shim to own the agent loop. Both bundle a native Claude Code binary — no separate CLI install is needed.

For all other languages, run the CLI programmatically via `claude -p` with `--output-format json` (see [headless mode](https://code.claude.com/docs/en/headless)).

### MCP SDK (building servers)

MCP servers can be built in any language with an MCP SDK:

| Language | SDK / Package |
|---|---|
| **Python** | `mcp` package |
| **TypeScript** | `@modelcontextprotocol/sdk` |
| **Java** | Spring AI MCP / `kotlin-sdk` |
| **Kotlin** | `kotlin-sdk` |
| **C#** | `ModelContextProtocol` |

### Claude Code surfaces (client implementations)

The Claude Code client itself (as an MCP host) runs on these surfaces:

| Surface | Language | SDK / Package |
|---|---|---|
| Terminal CLI | TypeScript (Node.js) | `@anthropic-ai/sdk`, `@modelcontextprotocol/sdk` |
| VS Code / JetBrains | TypeScript | Extension API + same SDK |
| Desktop app | TypeScript (Electron) | Same SDK |

MCP clients are transport-agnostic (JSON-RPC 2.0 over stdio or HTTP), so any language with HTTP support can act as an MCP client.

## Transport Layer

Claude Code supports two transport mechanisms:

### Stdio Transport

The server runs as a subprocess of the client. Communication happens over stdin/stdout. Used for local servers. No network overhead. Each connection is 1:1.

### Streamable HTTP Transport

The server runs as a remote HTTP endpoint. The client sends JSON-RPC messages via HTTP POST. Server-Sent Events (SSE) enable streaming responses and bidirectional notification support. Supports standard auth (bearer tokens, API keys, OAuth).

## Protocol Layer (JSON-RPC 2.0)

All MCP communication follows the JSON-RPC 2.0 specification. Messages fall into three categories:

- **Requests** (`id`, `method`, `params`) — expect a response
- **Responses** (`id`, `result` or `error`) — reply to a request
- **Notifications** (`method` only) — no response expected

## Lifecycle

### Initialization (Capability Negotiation)

Each time an MCP client connects to a server, it performs a handshake:

1. Client sends `initialize` with its capabilities and protocol version
2. Server responds with its capabilities and protocol version
3. Client sends `notifications/initialized` to confirm readiness

This negotiation determines which primitives (tools, resources, prompts) and features (list change notifications, elicitation) each side supports.

```
Client                              Server
  │── initialize request ──────────>│
  │<── initialize response ────────│
  │── notifications/initialized ──>│
```

### Teardown

Graceful shutdown via `shutdown` request, then transport-level disconnect.

## Server Primitives

MCP servers can expose these primitives. Claude Code uses them through its MCP clients:

### Tools

Executable functions the AI can invoke. Each tool has:
- `name` — unique identifier
- `description` — semantic purpose
- `inputSchema` — JSON Schema defining valid arguments

Flow: `tools/list` → `tools/call`

The LLM (Claude) decides which tools to call based on their descriptions. The MCP client executes the tool and returns the result to the LLM.

### Resources

Data sources providing context to the AI. Each resource has a URI and MIME type. Flow: `resources/list` → `resources/read`

Callers can also subscribe to resource change notifications.

### Prompts

Reusable templates for structuring AI interactions. Flow: `prompts/list` → `prompts/get`

## Client Primitives

Servers can use these to interact with the host:

- **Sampling** (`sampling/createMessage`) — server requests an LLM completion from the host
- **Elicitation** (`elicitation/create`) — server requests additional input from the user
- **Logging** (`logging/setLevel`) — server sends log messages to the client

## Notifications

Servers push real-time updates without polling:
- `notifications/tools/list_changed` — tools changed
- `notifications/resources/list_changed` — resources changed
- `notifications/prompts/list_changed` — prompts changed

On receiving a change notification, the client re-fetches the relevant list.

## Events Generated by Claude Code

Claude Code emits lifecycle events that a hook system or external orchestrator can consume. These are the events a LoopAI-MCP backend would need to intercept:

### Session Events

| Event | When | Fields |
|---|---|---|
| `SessionStart` | Session begins or resumes | `start_reason` (startup/resume/clear/compact/fork) |
| `SessionEnd` | Session terminates | `end_reason` (clear/resume/logout/etc.) |

### Turn Events

| Event | When | Fields |
|---|---|---|
| `UserPromptSubmit` | User submits a prompt | full prompt text |
| `Stop` | Claude finishes responding | turn summary |
| `StopFailure` | Turn ends with API error | `error_type` (rate_limit/overloaded/auth failed/etc.) |

### Tool Events (per tool call)

| Event | When | Fields |
|---|---|---|
| `PreToolUse` | Before a tool executes | `tool_name`, `tool_input` |
| `PostToolUse` | After a tool succeeds | `tool_name`, `tool_input`, `tool_output` |
| `PostToolUseFailure` | After a tool fails | `tool_name`, `tool_input`, `error` |
| `PostToolBatch` | After a parallel batch resolves | all batch results |
| `PermissionRequest` | Permission dialog shown | `tool_name`, `tool_input`, `mode` |
| `PermissionDenied` | Auto mode denies | `tool_name`, `reason` |

### Claude Code Built-in Tools

These are the tools Claude Code provides to the LLM out of the box. Any external orchestrator needs to understand them since the agent uses them on every task:

| Tool | Description | Requires Permission |
|---|---|---|
| `Read` | Read file contents | No (in project dir) |
| `Edit` | Targeted file edits | Yes |
| `Write` | Create or overwrite files | Yes |
| `Glob` | Find files by pattern | No |
| `Grep` | Search file contents by regex | No |
| `Bash` | Execute shell commands | Yes |
| `PowerShell` | Execute PowerShell commands (Windows) | Yes |
| `WebFetch` | Fetch URL content | Yes |
| `WebSearch` | Perform web search | Yes |
| `Agent` | Spawn a subagent | No |
| `Skill` | Execute a skill | Yes |
| `LSP` | Code intelligence | No |
| `Monitor` | Background command execution | Yes |
| `NotebookEdit` | Modify Jupyter notebook cells | Yes |
| `ToolSearch` | Discover/load MCP tools on demand | No |

### Subagent Events

| Event | When | Fields |
|---|---|---|
| `SubagentStart` | Subagent is spawned | `agent_id`, `agent_type` |
| `SubagentStop` | Subagent finishes | `agent_id`, `agent_type`, `result` |

### File / Workspace Events

| Event | When | Fields |
|---|---|---|
| `FileChanged` | Watched file changes on disk | `file_path` |
| `CwdChanged` | Working directory changes | `cwd` |
| `ConfigChange` | Config file changes during session | `source` |
| `WorktreeCreate` | Worktree created | `path` |
| `WorktreeRemove` | Worktree removed | `path` |

### Context Events

| Event | When | Fields |
|---|---|---|
| `PreCompact` | Before context compaction | `trigger` (manual/auto) |
| `PostCompact` | After compaction completes | — |
| `InstructionsLoaded` | CLAUDE.md or rule file loaded | `load_reason`, `file` |

### MCP Events

| Event | When | Fields |
|---|---|---|
| `Elicitation` | MCP server requests user input | `server_name` |
| `ElicitationResult` | User responds to elicitation | `server_name`, `response` |

## The Agent Loop

Claude Code's inner loop (abstracted for MCP interaction):

```
1. Receive prompt (user or system)
2. LLM evaluates state
3. LLM may call one or more MCP server tools (via the MCP client)
4. Tool results feed back to the LLM
5. Repeat until LLM produces text-only response
```

When the LLM calls an MCP tool:
1. The MCP client receives the tool call from the LLM
2. Sends a JSON-RPC request to the server (`tools/call`)
3. Server executes the tool
4. Server returns JSON-RPC response with content
5. Result is fed back to the LLM

## Tool Naming Convention for MCP Servers

When Claude Code connects to MCP servers, tools from those servers appear with prefixed names:

```
mcp__<server_name>__<tool_name>
```

For example:
- `mcp__filesystem__read_file` — from the filesystem MCP server
- `mcp__memory__create_entities` — from the memory MCP server

This applies to all hook events and permission rules.

## LoopAI-MCP Integration via Terminal I/O (PTY)

LoopAI-MCP does NOT use Claude Code's MCP protocol, Agent SDK, or plugin system for process control. Instead, the **launcher** allocates a PTY, spawns the client inside it, and controls it through terminal I/O — the same interface a human user would use. The client has no idea it is being driven programmatically.

### Architecture

```
                  ┌──────────────────────────────────────────┐
                  │          Go Backend (server)              │
                  │  - reads terminal output byte stream      │
                  │  - decides: type prompt / send Ctrl+C     │
                  │  - enforces compile → lint → test         │
                  │  - exposes MCP server with enforcement    │
                  │    tools for Claude's direct use          │
                  └────────────┬──────────────────┬───────────┘
                               │ HTTP/WS          │ MCP protocol
                               │ (control)        │ (tool serving)
                      ┌────────▼────────┐  ┌─────▼──────────┐
                      │    Launcher     │  │  Claude Code   │
                      │    (Go)         │  │  (LLM + tools) │
                      │                 │  │                │
                      │ allocates PTY   │  │ calls MCP      │
                      │ spawns client   │  │ tools on the   │
                      │ streams I/O     │  │ Go backend     │
                      │ detects idle    │  │ when needed    │
                      └────────┬────────┘  └────────────────┘
                               │ PTY master
                      ┌────────▼────────┐
                      │   Claude Code   │
                      │  (child proc)   │
                      │                 │
                      │ reads from PTY  │
                      │ writes to PTY   │
                      │ (thinks it's    │
                      │  talking to a   │
                      │  human user)    │
                      └─────────────────┘
```

### How it works

1. The launcher allocates a PTY (pseudo-terminal) and spawns `claude` as a child process connected to it
2. All terminal output from Claude Code (prompts, tool results, errors, ANSI escape codes) flows to the launcher
3. The launcher streams this output to the Go backend over HTTP/WS
4. The Go backend parses the byte stream to detect state (compiling, linting, testing, idle, error)
5. When the backend decides to act, it tells the launcher to type keystrokes into the PTY:
   - **Type a prompt:** `"fix the failing test in src/main.go"` — same as a user typing
   - **Send Ctrl+C:** interrupt a runaway tool call — the client handles it natively
   - **Send Ctrl+D or other control sequences** as needed
6. The client processes input as if from a real user, and its output flows back through the same PTY

### Idle Detection

The launcher detects the client is idle (ready for input) using a **timeout**: after N milliseconds of no output from the PTY, it assumes the client is waiting. No parsing of client-specific prompt strings — this is what makes the approach client-agnostic.

```
PTY output stops ──> timer starts
                    ──> timer fires (no output for N ms)
                        ──> launcher signals "idle" to backend
                            ──> backend decides next action
PTY output resumes ──> timer resets
```

### Prompt Injection

To inject a prompt, the backend tells the launcher to type it into the PTY, followed by Enter. The client receives it as if the user typed it — the same way Claude Code processes any user message.

```
Backend → launcher:  { type: "type", text: "fix the compile error in main.go" }
Launcher → PTY:      f  i  x     t  h  e     c  o  m  p  i  l  e  ...
                      e  r  r  o  r     i  n     m  a  i  n  .  g  o  \n
Claude Code sees:    "fix the compile error in main.go" (just like a user)
```

### Deny / Interrupt

To stop a tool call in progress (e.g., a runaway test suite), the backend tells the launcher to send Ctrl+C:

```
Backend → launcher:  { type: "ctrl_c" }
Launcher → PTY:      Ctrl+C character
Claude Code:         receives interrupt, cancels current tool, reports failure
```

### Enforcement Gate Flow (Example)

```
1. Launcher spawns Claude Code with prompt: "add feature X"
2. Claude starts working — terminal output streams to backend
3. Backend sees Claude call Bash to run `go build`
4. Build output streams — backend parses it
5. Build fails — terminal output stops
6. Idle timeout fires — launcher signals idle
7. Backend: "inject prompt: compile error, fix it and rerun"
8. Launcher types the prompt into the PTY
9. Claude receives it, reads the file, fixes the code, re-runs
10. Loop continues until all gates pass
```

### Relationship with MCP

The Go backend also exposes a standard MCP server with enforcement tools (`compile_project`, `run_lint`, `run_tests`). When Claude Code needs to compile or lint, it can call these tools directly. Their output appears in the terminal just like any other tool call, and the backend sees it in the PTY stream.

```
Two independent communication channels:

1. PTY I/O (launcher ↔ client process):
   Carries all terminal output and keystrokes.
   The backend drives the loop through this channel.

2. MCP protocol (Claude Code LLM ↔ backend):
   Standard MCP, carries tool invocations only.
   Optional — enforcement can also work purely through PTY I/O.
```

## Deployment

### Launcher replaces the CLI

The user runs `loopai` (the launcher), not `claude` directly. The launcher spawns the real client as a child process inside a PTY.

```
go install github.com/romayengineer/loopai-mcp/launcher/...@latest
loopai "build the auth module"
# internally: allocates PTY → spawns `claude` → streams I/O to backend
```

### Go backend is a separate binary

```
go install github.com/romayengineer/loopai-mcp/backend/go/cmd/...@latest
# or
brew install loopai-mcp/tap/loopai-backend
```

```
loopai-backend
# starts on :8090 by default
```

### How they connect

The launcher discovers the backend through configuration:

| Method | Config |
|---|---|
| Env var | `LOOPAI_BACKEND_URL=http://localhost:8090` |
| CLI flag | `loopai --backend http://localhost:8090 "prompt"` |
| Config file | `~/.config/loopai/config.toml` `[backend] url = "http://localhost:8090"` |

### Workflow

```
terminal 1 $ loopai-backend
             ← Go backend listening on :8090

terminal 2 $ loopai "refactor the auth module"
             ← launcher allocates PTY
             ← spawns `claude` inside PTY
             ← streams terminal output to backend
             ← backend makes decisions
             ← launcher types prompts / sends Ctrl+C as instructed
```

### Why the launcher and backend are separate

| Component | Language | Runs where | Responsibility |
|---|---|---|---|
| Launcher | Go | Developer machine, CI | PTY lifecycle, I/O streaming, idle detection |
| Backend | Go | Developer machine, CI, server | Decision-making, enforcement, state machine |

Same language (Go) for both. The backends's language is the launcher's language — no cross-language dependency.
