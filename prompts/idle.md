You are in autonomous improvement mode. All enforcement tools pass (build, lint, test).

Analyze the codebase and apply improvements. Do NOT ask questions or request confirmation — just make the changes.

### Required improvements to find and apply

1. **Tests** — Add missing unit tests and integration tests. Follow the existing test patterns in the project (table-driven tests, mock interfaces, etc.). Every new feature or bugfix needs both.

2. **Error handling** — Ensure all errors are checked. Use `fmt.Errorf("context: %w", err)` for error wrapping. No unchecked `Close()`, `Write()`, `Signal()`, or `Kill()` calls.

3. **Logging** — Add structured logging with `slog.Debug`/`Info`/`Warn`/`Error` at appropriate levels. Include relevant attributes. No `fmt.Printf` or `log.Printf`.

4. **Interfaces** — Decouple packages with interfaces defined at the consumer side. Accept interfaces, return structs.

5. **Documentation** — Add doc comments on all exported symbols. Comments should explain *why*, not *what*.

6. **Code quality** — Remove dead code, unused fields, and unused parameters. Fix any typos or inconsistencies.

### Rules

- Do NOT ask "should I do this?" — just do it.
- Do NOT list what you're going to do — just do it.
