# S14 Implementation Plan — Terminal UI

Status: delivered (2026-11-06)
Scope: [../14-tui.md](../14-tui.md)
Normative source: [../../PLAN.md](../../PLAN.md) §3.10, §3.11, §3.13.

## 0. Principles

- The TUI is a view over the same core services. It adds no domain rule and no second book
  implementation.
- The CLI stays the complete tooling surface; automation, agents and CI never need the TUI.
- Rendering is deterministic ASCII; the view models are tested without a terminal.
- Errors are the core's `cli.Error` codes, shown verbatim in the status line.
- No new dependency: the renderer and input handling use the standard library only.

## 1. Delivery slices

| Slice | Content | Why |
|---|---|---|
| **S14a** | View model, intents, key map, ASCII view renderer and their tests | Pure, testable UI state |
| **S14b** | Controller over the core services, `pecia tui` entry, TTY guard, parity test | The interactive loop |
| **S14c** | `$EDITOR` handoff, sync/check presentation, lock/stale recovery, key-map doc | The full round-trip |

## 2. Decisions

### D1 — No TUI library

The renderer is hand-rolled over `bufio`/`fmt`; ANSI is limited to cursor movement, clear-screen and
raw mode. The view returns plain ASCII lines, so tests never parse escape sequences. No new module.

### D2 — Entry and TTY guard

`pecia tui [--book <id>]` is the only entry. It requires both stdin and stdout to be a TTY; otherwise
it fails with `tui.no-terminal` and a hint to use the CLI. There is no silent non-interactive mode.
`--book` is optional; without it the TUI shows a book picker from the registry. The first loop reads
one command per line (`up`, `down`, `enter` or a single key) and redraws the screen; raw keypress
input is a possible follow-up, not a requirement.

### D3 — Layout

One screen, fixed proportions: a header line (book, mode, lock state), a left tree pane (40%), a
right body pane (the selected body or the node description), a status line and a one-line key hint.
Panes are not resizable. Lines are padded to the terminal width and clipped, never wrapped.

### D4 — Editing

Body editing hands the terminal to `$EDITOR` (or `notepad` on Windows when unset); on return the text
is normalized and written through `body.Service`. Node titles are edited inline with a one-line
prompt. Structure creation uses the core (chapter/subchapter/block) and surfaces the core's errors.

### D5 — Actions

Open, close, tree navigation, create chapter/subchapter (inline title), create block and edit its
body (`$EDITOR`), edit the selected body, move a block up/down within its parent, sync, check, reload
and quit. Grammar findings are shown from `check`; applying them stays a CLI action. Status display
reads the existing `status`/`sync` payloads in-process.

### D6 — Book picker

Without `--book`, the picker lists the registry entries in library order with their open state.
Enter opens a closed book and shows it; `q` exits.

### D7 — Errors and recovery

Every core error is rendered as `<code>: <message>` in the status line. A locked or stale resource is
recoverable: the TUI keeps the editor buffer, offers `r` (reload) and does not overwrite input. No
domain error is re-implemented.

### D8 — Model and controller split

The model is pure: a snapshot in, key events in, state plus an `Intent` out. The controller maps
intents to core calls and rebuilds the snapshot. A parity test enumerates every intent and fails when
one has no controller mapping.

### D9 — Tests

Model tests cover navigation, the view lines, the key map and the picker; controller tests cover the
same round-trip as the CLI acceptance test (create, body, move, sync, close) with a fixture book;
no test needs a TTY. `go test -race` stays clean.

## 3. S14a tasks

Delivered 2026-11-06.

1. `internal/tui`: `Snapshot`, `Node`, `Intent`, `Key`, `Model` and `Handle`.
2. The ASCII `View(width, height)` with the header, tree, body, status and key hint.
3. The key map table and the model tests (navigation wrap, selection, view content, intents).
4. Package doc and no CLI/core import in the view files.

## 4. S14b tasks

Delivered 2026-11-06.

1. The controller: open/close, navigation reloads the body, creation and move through `structure`,
   body writes through `body`, sync/check through `render`.
2. `pecia tui` with the TTY guard and the raw-mode loop; book picker.
3. The parity test (every intent has a controller call) and the controller round-trip test.

## 5. S14c tasks

Delivered 2026-11-06.

1. `$EDITOR` handoff and the inline title prompt.
2. Status-line presentation of sync/check findings, lock/stale recovery and reload.
3. A short key-map note under `Docs/` and the scope/status documentation.

## 6. Acceptance mapping

| # | Criterion | Slice |
|---|---|---|
| 1 | Fixture round-trip: create node, write body, move block, sync, close | S14b |
| 2 | Every action maps to a core call (parity test) | S14b |
| 3 | No domain rules in the TUI (core errors surface) | S14a/S14b |
| 4 | Locked/stale resource visible and recoverable, no lost input | S14c |
| 5 | No TTY: clear error, no interactive fallback | S14b |
| 6 | Tests pass without a TTY; `-race` clean | S14a–S14c |

## 7. Verification

- `gofmt -l .`, `go vet ./...`, `go build ./...`, `go test ./... -count=1`, `go test -race ./...`,
  then `.\pecia.exe sync --check` (must be clean).
