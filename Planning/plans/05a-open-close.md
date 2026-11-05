# S05a — Implementation Plan: Open/Close Mechanics

Status: implemented (2026-11-05). Parent scope: [../05-open-close-and-rollback.md](../05-open-close-and-rollback.md).
Normative source: [../../PLAN.md](../../PLAN.md) §3.5, §3.7 (open state, extracted bodies), §6.

First delivery slice of S05. S05b adds the rollback tiers and the reported crash state.

## 1. Decisions

| # | Question | Decision | Rationale |
|---|---|---|---|
| 1 | State file | `<store>/open.json`, `{"version":1,"books":[{id, outputPath, pid, session, openedAt}]}` | `PLAN.md` §3.7: one file keyed by book ID; a random session plus the pid survives pid reuse |
| 2 | Empty state | the file is removed when the last entry goes | "Not open" is the absence of an entry |
| 3 | Extracted set | `manifest.json` plus `blocks/<id>.md`; `layouts/` is durable store data, not a container member | Keeps the container to structure and bodies, matching the store split in §3.2 |
| 4 | Authority | extracted files are authoritative while open; close rebuilds from them | `PLAN.md` §3.5 |
| 5 | Validation | close decodes, validates and encodes before it writes anything | A broken book must not reach the container |
| 6 | Reject | a validation failure at close is a `book.invalid` reject (exit 3) with the issues | "A gate that blocks is an informing reject" |
| 7 | Empty bodies | a placed block without a file is written as an empty body; a body file without a placement is an orphan and rejects the close | A freshly placed block has no prose yet; an orphan is a real inconsistency |
| 8 | Recovery | in place, no repair command: extracted without entry = `unsaved` (adopt on open, repack on close); entry without extracted = `orphaned` (re-extract on open, drop the entry on close) | `PLAN.md` §3.5: report, no separate recover path |
| 9 | Locking | open/close take the store and the book exclusively; the book lock is the gate. Body writes (S08) take the book shared and the block exclusively; structure changes take the book shared and the node exclusively | Exclusive close waits for every shared holder, so no body write can be in flight during a repack |
| 10 | Determinism | open then close without edits re-saves the identical container bytes | The input hash must not change just because a book was opened |
| 11 | Output path | read from the registry at open and recorded in the entry | `PLAN.md` §3.7 records the output path; S11 owns the registry |
| 12 | Commands | `book open|close|status`, all with the shared output contract; open and close support `--dry-run` | The lifecycle is usable without S07/S08 |
| 13 | Error mapping | domain errors expose `ErrorCode()`; the CLI keeps the code and message | One adapter instead of per-command mapping |
| 14 | Immovability | `RequireClosed` is the guard S11 calls before move, rename or unregister | The lifecycle owns the predicate, the library command owns the operation |

## 2. Error codes

`lifecycle.no-container`, `lifecycle.extracted-incomplete`, `lifecycle.state-corrupt`,
`lifecycle.state-version`, `lifecycle.not-open`, `lifecycle.open`, `lifecycle.read-failed`,
`lifecycle.write-failed`, `lifecycle.encode-failed`, plus `book.invalid` as the reject code.

## 3. Files

```
internal/lifecycle/lifecycle.go   Status, Entry, State, Open, Close, RequireOpen/Closed, repack
internal/lifecycle/state.go       open.json read/write/upsert/remove
internal/lifecycle/command.go     book open|close|status, payloads, dry-run receipts
internal/lifecycle/*_test.go      lifecycle and command tests
internal/coordinator/*            Shared/Exclusive mode
internal/store/layout.go          Layout.OpenState()
internal/store/state.go           WriteAtomic()
internal/cli/errors.go            coded domain errors
```

## 4. Acceptance

| S05 criterion | Evidence |
|---|---|
| 1 open extracts, closed is not readable | extraction test, `RequireOpen` error |
| 2 extracted data authoritative | edit-then-close test |
| 3 close repacks deterministically | byte-identical round trip |
| 4 idempotence | double open keeps one entry; close without open is a no-op |
| 5 multiple books | two open books, closing one leaves the other |
| 6 state tracked | `open.json` entry with path, pid, session, timestamp |
| 9 crash state detected | `unsaved` and `orphaned` states and their in-place recovery |
| 11 immovable guard | `RequireClosed` error |
| 13 close excludes writers | exclusive book lock against shared holders |

## 5. Out of scope

- Undo packages and book-level rollback points: S05b.
- `check`/`verify` reporting an open book as a finding: S09 wire-up (S05b provides the predicate).
- Enforcing immovability in `library` commands: S11.
- Daemon backend: S06b.
- Body writes and structure commands: S07, S08.
