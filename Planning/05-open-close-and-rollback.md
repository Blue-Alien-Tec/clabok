# S05 — Open/Close Lifecycle and Rollback Tiers

Status: implemented (2026-11-05). Delivery slices: S05a (open/close mechanics, open state,
extraction, repack, recovery, delivered) and S05b (rollback tiers, crash reporting, delivered).
Phase: 3. Depends on: S04 (container, store layout), S06 (coordinator interface). Blocks: S07, S08,
S11, S14.
PLAN refs: §3.5 (complete lifecycle), §3.7 (open state, extracted bodies, rollback, immovability),
§6 (open/close safety, rollback for every write).

## 1. Goal

Define and enforce the book lifecycle: a closed book is a packed container, an open book is extracted
authoritative body files, every writer creates an undo package, closing repacks deterministically,
and a crash is reported instead of repaired.

## 2. In scope

| # | Requirement | PLAN |
|---|---|---|
| 1 | `open <book>` extracts all body data; every body read and write requires an open book; a closed book is not readable | §3.5 |
| 2 | Extracted body files are authoritative while open; the container is not consulted until close | §3.5 |
| 3 | `close <book>` repacks deterministically; closing an unchanged book creates no book-level rollback point | §3.5 |
| 4 | Idempotence: `open` on an open book and `close` on a closed book succeed without side effects; errors are reserved for real errors | §3.5 |
| 5 | Multiple books may be open; open and close take an explicit book and never touch another open book; a second `open` creates no second extracted state | §3.5 |
| 6 | Open state is tracked, not inferred: a state file keyed by book ID records output path, pid or session, and timestamp | §3.7 |
| 7 | Every mutating command creates an undo-capable rollback package; undo packages are capped (default 10, S02) and deleted when the book closes | §3.5, §3.7 |
| 8 | Closing creates a book-level rollback point only if the book changed; at most 3 are kept, oldest evicted (limits from S02) | §3.5 |
| 9 | Crash state: when extracted changes are newer than the container, report the state and deliver the readable content directly; no separate recover or repair command | §3.5 |
| 10 | `check` and `verify` report an open book as a non-blocking `open-book` finding that does not change the exit code; `book status` shows the open state; `sync --check` stays a drift check | §3.5 |
| 11 | An open book cannot be moved, renamed or unregistered | §3.7 |
| 12 | An interrupted open or close is detectable and never corrupts the container | §6 |
| 13 | Close coordinates with in-flight block edits: it waits for or rejects them and never repacks a body being written | §3.12 (coordination owned by S06) |
| 14 | The lifecycle works under both coordination backends with identical semantics | §3.5, §3.12 |

## 3. Out of scope

- Lock primitives, backends and lock ordering: S06.
- Container codec, checksums and atomic writes: S04.
- Which commands mutate and therefore need an undo package: the owning scopes (S07, S08, S12) call
  the rollback API provided here.
- Book settings and structure commands: S07.
- Renderer and `sync --write`/`--prune`: S09.
- Registry entry updates on move/rename: S11 (this scope only enforces the refusal).

## 4. Deliverables

- `internal/store` lifecycle (or a dedicated lifecycle package): open, close, state file, extraction,
  repack, change detection.
- `open.json` state codec with atomic replace and a recovery view for a stale entry.
- Rollback API: create undo package, list, restore, evict, delete-on-close, book-level point with cap.
- `book` command subgroup for `open`, `close`, `status` (settings stay in S07).
- `open-book` finding integration point used by `check`/`verify`.
- Tests: crash simulation (process killed between extract and close), double open, close without
  open, close without changes, cap enforcement, eviction order, immovability refusals.
- This scope's implementation plan under `Planning/plans/`.

## 5. Acceptance criteria

1. `open` twice in two processes results in one extracted state and one state entry; the second call
   is a no-op.
2. A body read on a closed book fails with a stable `book.not-open` code; no fallback to the
   container.
3. `close` after a body edit repacks the change; `sync --check` equivalents see the new state.
4. `close` on an unchanged book writes no book-level rollback point; the container is byte-identical.
5. Undo packages stop at the configured limit and are removed by `close`; book rollback points stop
   at three and evict the oldest.
6. Killing the process after an extraction but before a close reports the crash state on the next
   command and delivers the extracted content read-only through the normal pipeline.
7. A corrupt or half-written state file is detected and reported; the container is still readable.
8. Moving, renaming or unregistering an open book is refused with a stable code.
9. `check` reports `open-book` without changing the exit code; `book status` shows the open state.
10. An interrupted close (kill between repack and state cleanup) leaves a readable container and a
    state that the next command can reconcile without a repair command.

## 6. Design decisions for this scope's planning session

- `open.json` shape and atomicity; whether the state file lives per store or per book folder.
- Session identity: pid plus start time, a session token, or both. How a dead pid is recognized on
  Windows and Unix, and whether takeover is automatic or requires an explicit `open` after a report.
- What "changed" means for the book-level rollback point (manifest hash, body hash, or the S04 hash
  pair).
- Undo package content and naming; whether a package is a full snapshot or a delta, and how restore
  interacts with an open book.
- Whether close is refused while another process holds a block lock, or waits (S06 provides the
  primitive; the policy is decided here).
- Does `close` prune an empty undo directory, and does `open` delete a stale extracted folder from a
  previous crash or keep it for inspection?
- Where the readable-content-delivery path surfaces: `book status`, `check`, or both.
- Whether `open` allocates the book's output path in the registry (coordination with S11).

## 7. Risks and constraints

- Crash handling is the hardest part. Model it with a test that can kill the process at defined
  points instead of reasoning about it.
- The lifecycle must not become a hidden second storage mode. `PLAN.md` explicitly replaces the
  storage-mode setting with open/close; there is no `--storage` flag.
- Rollback is the safety net for every write. A missing package on an unexpected error path is a
  defect, not a nicety; test the error path, not only the success path.
