# S05b — Implementation Plan: Rollback Tiers and Crash Reporting

Status: implemented (2026-11-05). Parent scope: [../05-open-close-and-rollback.md](../05-open-close-and-rollback.md).
Normative source: [../../PLAN.md](../../PLAN.md) §3.5, §3.7 (rollback), §6.

Second and last delivery slice of S05.

## 1. Decisions

| # | Question | Decision | Rationale |
|---|---|---|---|
| 1 | Package location | `<store>/rollback/<bookId>/`, one ZIP per package | Fixed store layout; packages are per book |
| 2 | Package format | the same verified member container as a book (checksums, name rules, atomic save) | No second archive codec; corruption is detected on load |
| 3 | Package name | `<kind>-<timestamp>-<reason>.zip`, timestamp `20060102T150405.000000000Z` | Lexicographic order equals age; kind and reason are parseable |
| 4 | Timestamps | monotonic within one process; a coarse clock cannot produce two equal names | Two snapshots in the same tick must not overwrite each other |
| 5 | Kind limits | `undo` capped by `rollback.undoLimit` (default 10), `book` by `rollback.bookPointLimit` (default 3); `0` disables the kind | Config from S02; a disabled pool writes nothing |
| 6 | Undo creation | `lifecycle.Snapshot(id, reason)` captures the extracted member set; the caller holds the book lock | A snapshot inside the caller's lock scope cannot self-deadlock |
| 7 | Book-level point | created at close only when the packed bytes differ from the current container; the old member set is stored | `PLAN.md`: only if the book changed |
| 8 | Undo lifetime | cleared on close (best effort) | `PLAN.md`: undo packages are deleted when the book closes; a failed cleanup must not fail a successful close |
| 9 | Restore | snapshots the current state first, then applies; undo restores the extracted state (book must be open), book restores the container (book must be closed) | Every write stays undoable; the kind dictates the target |
| 10 | Restore target | a full state replacement: the extracted `blocks/` and `manifest.json` are removed and rewritten | No stale body survives a restore |
| 11 | Commands | `rollback list <book>`, `rollback restore <book> <package>`; restore is mutating and supports `--dry-run` | Inspectable and reversible from the CLI |
| 12 | Immovability and findings | `RequireClosed` and `State` are the predicates S11 and S09 use; no duplicate logic | The lifecycle owns the state, the commands own the operation |

## 2. Error codes

`rollback.invalid-root`, `rollback.invalid-limit`, `rollback.invalid-kind`,
`rollback.invalid-package`, `rollback.read-failed`, `rollback.write-failed`, plus
`lifecycle.not-open`, `lifecycle.open`, `lifecycle.extracted-incomplete` from S05a.

## 3. Tasks

1. `internal/rollback`: store, snapshot, list, load, clear, prune, naming.
2. Lifecycle: `Options`, `Snapshot`, `Restore`, book point at close, undo cleanup, member helpers.
3. `book close` payload reports `bookPoint`.
4. CLI: `rollback list`, `rollback restore` with dry-run.
5. Tests: package unit tests, undo/point caps, restore both ways, command round trip.
6. `gofmt`, `go vet`, `go build`, `go test`.

## 4. Acceptance

| S05 criterion | Evidence |
|---|---|
| 7 undo package per write, capped, deleted on close | snapshot test, limit test, clear-on-close test |
| 8 book point only when changed, capped | unchanged-close test, change-close test, cap test |
| 9 crash state reported without a repair command | S05a recovery tests and `book status` |
| 11 immovable guard | `RequireClosed` test |
| 13 close excludes writers | exclusive book lock against shared holders (S06a/S05a) |

## 5. Out of scope

- `check`/`verify` printing the `open-book` finding: S09 wires it against `lifecycle.State`.
- Enforcing immovability in `library` commands: S11.
- Daemon backend: S06b.
- Undo packages for the structure commands: S07 and S08 call `Snapshot` before their writes.

## 6. Implementation notes (2026-11-05)

- The store lock inside the container codec is reused for packages, so a package is verified on
  restore without a second format.
- `Snapshot` takes no lock by design; the command that mutates already holds the book lock. Taking
  the book exclusively inside `Snapshot` would self-deadlock against the caller's shared lock.
- Close records the book point before saving the new container, so a snapshot failure leaves the
  old container untouched.
- A package restore snapshots the pre-restore state with the same kind, which makes a restore
  itself reversible.
- `rollback list` shows newest first; the pool is stored oldest first for pruning.
