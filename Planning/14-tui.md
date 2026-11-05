# S14 — Terminal UI

Status: draft, not planned.
Phase: 8. Depends on: S07 (structure), S08 (body), S09 (render), plus S05/S06 behaviour at runtime.
Blocks: none.
PLAN refs: §3.10 (TUI included, one tooling surface), §3.13 (package responsibility), §3.11 (config).

## 0. Current state and pre-planned decisions (2026-11-05)

- The CLI surface is complete for authoring and rendering; every payload implements `Text()` and
  mutating receipts implement `DryRunResult()`.
- The core services are plain Go (`structure`, `body`, `render`, `lifecycle`, `search`, `temp`) and
  can be called in-process.
- The relevant rules are one static binary and no new dependency without an owner decision.

Pre-planned decisions:

- Entry `pecia tui`, only when stdin and stdout are a TTY; otherwise a clear error, never an
  interactive fallback in pipelines.
- No TUI library without an explicit owner decision; the default is a hand-rolled renderer over the
  existing services.
- Panes: structure tree, body editor/view (external editor or inline), status line, sync/check.
- Core errors surface as `cli.Error` codes; the TUI adds no domain rules.
- View models are tested without a terminal.

## 1. Goal

Give humans an interactive terminal surface on the same core services, without a second
implementation of any book behaviour and without making the CLI interactive.

## 2. In scope

| # | Requirement | PLAN |
|---|---|---|
| 1 | The TUI ships in the same static binary and uses the same core services; no domain logic in the UI package | §3.10 |
| 2 | The CLI remains the complete tooling surface; automation, agents and CI use the CLI, never the TUI | §3.10 |
| 3 | The TUI is the only interactive surface: no prompts in the CLI commands | §3.10 |
| 4 | Core operations reachable: open/close, structure navigation and editing, body editing, settings, sync/check, status | §3.10 |
| 5 | Behaviour under errors is the core's behaviour: stable error objects are shown, not re-implemented; locked or stale resources surface as a recoverable state | §3.12 |
| 6 | Deterministic, ASCII-only rendering; no dependency on terminal features beyond common ANSI capabilities; works in Windows Terminal and common Unix terminals | §3.10 |
| 7 | Editing a body either opens the external editor or uses an inline editor; the choice is a design decision, not a product statement | §3.10 |
| 8 | View models are testable without a terminal (input events -> state -> output lines) | §6 (test suite) |

## 3. Out of scope

- Any new capability that the CLI does not expose.
- GUI, web UI or remote UI.
- Replacing the CLI in CI or in agent workflows.
- The daemon's server mode: S06.
- Config semantics: S02.

## 4. Deliverables

- A TUI package with view models, input handling and a renderer over the core services.
- Wiring in `cmd/pecia` (a subcommand or a mode, decided in planning) that does not add logic to
  `main`.
- View-model tests: navigation, editing, error surface, lock conflict, no-terminal fallback.
- A short user-facing key map (authored with CLABOK once S11 has landed; a plain Markdown note
  before that).
- This scope's implementation plan under `Planning/plans/`.

## 5. Acceptance criteria

1. The TUI performs a full round-trip on a fixture book: create node, write body, move block, sync,
   close.
2. Every TUI action maps to an existing core operation; a parity test enumerates the actions and
   fails when one has no core call.
3. No domain rule (level limit, heading rule, block-ID rule) is implemented in the TUI package;
   violations surface as core errors.
4. A locked or stale resource produces a visible, recoverable state and no lost input.
5. The TUI runs without a terminal (piped output) by falling back to a plain mode or a clear error.
6. Package tests pass in CI without a TTY; `go test -race` remains clean.

## 6. Design decisions for this scope's planning session

- TUI library: standard library only versus a dependency. `PLAN.md` requires one static binary, not a
  zero-dependency UI; a dependency is an owner decision (see `Planning/README.md` §8).
- Entry point: `pecia tui` versus a mode when both stdin and stdout are a TTY. The latter is
  convenient but must never trigger under automation.
- Layout: tree pane, body pane, status line; and whether panes are resizable.
- Editing: inline editor versus `$EDITOR` handoff. The second is simpler and reuses the author's
  editor; the first is faster for small bodies.
- How sync results and findings are presented, and whether the TUI can apply grammar findings.
- Whether the TUI requires an open book or can open one from a picker.

## 7. Risks and constraints

- A TUI that duplicates logic is the failure mode of this scope. It must be a view over the core.
- Hidden interactivity breaks automation. Any auto-detection of a TTY must be explicit and
  overridable.
- Terminal rendering differs between Windows and Unix; test the rendered lines, not the escape
  sequences.
- The TUI is the last phase. It must not block M5 or any earlier milestone.
