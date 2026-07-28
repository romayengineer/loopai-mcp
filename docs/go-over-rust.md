# Why Go over Rust for LoopAI-MCP

## Project requirements that shaped the decision

LoopAI-MCP has two Go binaries with distinct roles:

- **Launcher** — allocates a PTY, spawns a TUI client as a child process, streams bytes between the PTY and the Go backend over HTTP/WebSocket, detects idle via output timeout
- **Backend** — reads the terminal output stream through the launcher, runs a state machine (detecting compile/lint/test phases), decides when to type prompts or send Ctrl+C

The core challenge is **I/O orchestration**, not CPU-bound computation. Every operation is bounded by network latency, terminal output speed, and human-perceptible time scales (hundreds of milliseconds to seconds).

## Comparison by project-relevant criteria

### Concurrency model

The launcher's inner loop has three concurrent concerns:

```
1. Read from PTY master → forward to backend (HTTP/WS)
2. Read from backend → write to PTY master (keystrokes, Ctrl+C)
3. Idle timer: reset on PTY output, fire after N ms of silence
```

**Go** maps this directly to three goroutines communicating over channels. The launcher is a few hundred lines of straightforward synchronous code per goroutine.

```go
// Go — three goroutines, shared state via channels
go ptyReader(ptym, sendCh)    // reads PTY → sends to backend
go backendReader(recvCh, ptym) // reads from backend → writes to PTY
go idleTimer(ptym, timeout)    // fires on no-output
```

**Rust** requires `tokio::select!` in a single async task, or `tokio::spawn` with `mpsc` channels. Both work, but the explicit async wiring adds complexity without benefit for this simple fan-in/fan-out pattern. Rust's zero-cost abstractions shine when you need maximum throughput — but the bottleneck here is the terminal and network, not the multiplexing code.

### PTY ecosystem

| Criterion | Go | Rust |
|---|---|---|
| Library | `creack/pty` | `portable-pty` |
| Maturity | 7+ years, production-tested | Production (WezTerm), actively maintained |
| Platforms | macOS, Linux, Windows (ConPTY) | macOS, Linux, Windows |
| API | Simple `os.File`-like interface | Stream-based, async-friendly |

Both are adequate. Go's `creack/pty` returns a standard `*os.File` that works with `io.Copy`, `bufio.Scanner`, and every Go I/O utility — no adapters needed.

### Terminal output parsing

The backend needs to parse ANSI escape codes from raw terminal output to detect state. This is the one area where Rust has a clearly stronger ecosystem.

| Language | Libraries |
|---|---|
| Rust | `alacritty_terminal` (production-grade vte parser), `termwiz`, `vte` |
| Go | `gdamore/tcell` (emulator, not a parser), `golang.org/x/term` (raw mode only) |

**However**, the project does not need full terminal emulation. It needs to detect a small number of patterns in the output:
- Is the client still writing output? (idle detection does this purely by timeout)
- Did a compile/lint/test command produce error output? (keyword matching on raw bytes)
- Has the client presented a prompt? (timeout + output stabilization)

These are solvable with a simple state machine over byte streams, not a full VT500 parser. If full parsing becomes necessary later, Go can use `tcell` or a purpose-built VTE parser of a few hundred lines.

### Build and deployment

| Criterion | Go | Rust |
|---|---|---|
| Cross-compile | `GOOS=linux GOARCH=arm64 go build` | `rustup target add aarch64-unknown-linux-gnu`, then `cargo build --target ...` |
| Compile speed | Seconds | Minutes for release builds |
| Binary size | ~15 MB (static) | ~5 MB (with LTO) |
| Runtime | Self-contained binary | Self-contained binary |

Both produce static binaries with no runtime dependency. Go's cross-compilation is notably simpler — a single environment variable, no target toolchain installation.

### Team and maintenance

| Criterion | Go | Rust |
|---|---|---|
| Learning curve | Low — simple language, familiar syntax | High — borrow checker, lifetimes, ownership model |
| Compiler errors | Helpful, actionable | Verbose, can be daunting |
| Talent pool | Larger | Smaller, growing |
| Iteration speed | Fast — compile in seconds, run immediately | Slower — compile times grow with project size |

For an open-source project where contributors may have varying experience levels, Go's simplicity reduces the barrier to contribution.

## Decision summary

**Go is the right choice** because the project's bottleneck is architecture and iteration speed, not CPU performance.

The one area Rust genuinely wins — terminal escape code parsing — is not a deciding factor because:
1. Idle detection is purely timeout-based, not parser-based
2. State detection (compile/lint/test output) can be done with keyword matching on raw bytes
3. If full ANSI parsing becomes necessary, it's implementable in Go or available via libraries

Both languages would produce a working system. Go produces it faster, with simpler code, easier cross-compilation, and a lower barrier for contributors — without sacrificing measurable runtime performance for this I/O-bound workload.
