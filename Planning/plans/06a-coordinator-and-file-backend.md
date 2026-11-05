# S06a — Implementation Plan: Coordinator Interface and File Backend

Status: implemented (2026-11-05). Parent scope: [../06-concurrency-and-coordination.md](../06-concurrency-and-coordination.md).
Normative source: [../../PLAN.md](../../PLAN.md) §3.12, §6.

First delivery slice of S06. S06b adds the daemon backend (`pecia serve`), mode selection, the
version handshake and the shared conformance suite.

## 1. Decisions

| # | Question | Decision | Rationale |
|---|---|---|---|
| 1 | Interface | `Acquire([]Resource, mode, timeout) (Release, error)` with an explicit resource set and a shared/exclusive mode | Multi-lock operations are the normal case (a move touches two nodes); close needs an exclusive gate while body writes hold the book shared |
| 2 | Resource identity | `{Kind, Book, ID}` with kinds `book`, `node`, `block`; node and block need a nonzero ID, a book must not carry one | Mirrors the model's identity and the lock granularity in `PLAN.md` |
| 3 | Ordering | the coordinator sorts resources by rank (book, node, block), then book ID, then element ID; duplicates collapse | Every process acquires in the same order, so crossing moves cannot deadlock |
| 4 | Lock primitive | OS advisory lock: `flock` on Unix, `LockFileEx` on Windows via `syscall` | The OS releases it on process death: no stale cleanup, no deletion race (same reasoning as the DocEngine reference) |
| 5 | Lock files | `<lockDir>/book-<id>.lock`, `node-<book>-<id>.lock`, `block-<book>-<id>.lock`; the directory is `Layout.Coordinator()` = `<store>/.coordinator` | One file per resource, stable names, inside the store |
| 6 | Stale state | none tracked; a dead process releases its locks | The only reliable stale signal is the OS |
| 7 | Holder metadata | pid, start time, command and the resource, written behind the locked byte | A waiting process can name the holder in the busy message |
| 8 | Timeout | default 5 s, per call overridable; retry with 50 ms doubling to 250 ms | Bounded waiting with a clear failure instead of a hang |
| 9 | Reads | no lock; a reader relies on the store's atomic replacement | `PLAN.md`: body reads use an atomic snapshot |
| 10 | Release | idempotent; the lock file stays on disk | Deleting a lock file while another process has it open recreates the race the OS lock avoids |
| 11 | Empty resource set | succeeds with a no-op release | A command that locks nothing must not fail |
| 12 | Dependencies | standard library only (`syscall`), no `x/sys` | Single static binary, no new module dependency |

## 2. Error codes

`coordinator.invalid-resource`, `coordinator.busy`, `coordinator.unsupported`, `coordinator.io`.

## 3. Files

```
internal/coordinator/coordinator.go   Resource, Kind, ordering, Error, interface
internal/coordinator/file.go          File backend, holder metadata, acquire/release
internal/coordinator/lock_unix.go     flock
internal/coordinator/lock_windows.go  LockFileEx / UnlockFileEx
internal/coordinator/lock_other.go    unsupported platforms
internal/coordinator/file_test.go
internal/coordinator/coordinator_test.go
internal/store/layout.go              + Layout.Coordinator()
```

## 4. Tasks

1. Resource identity, validation, ordering and deduplication.
2. `Coordinator` interface and `Release`.
3. `Error` with stable codes.
4. File backend: open, lock with timeout and backoff, holder metadata, release.
5. Platform lock implementations and the unsupported fallback.
6. `Layout.Coordinator()` and the layout test.
7. Tests: busy, different resources, ordering, release idempotence, holder text, invalid resources.
8. `gofmt`, `go vet`, `go build`, `go test`.

## 5. Acceptance

| S06 criterion | Evidence |
|---|---|
| 1 one writer per locked resource | two backends, same resource, second is busy |
| 2 per-block concurrency, per-node serialization | different block resources succeed concurrently |
| 3 stable lock order, no deadlock | ordering test and a two-goroutine crossing test |
| 5 file backend with stale detection | OS lock; a released lock is immediately free |
| 12 no data difference | the backend touches no book data |

## 6. Out of scope

- `pecia serve`, the daemon transport, mode selection and the version handshake: S06b.
- The shared conformance suite across both backends: S06b.
- Which command takes which lock: S05, S07, S08, S12.
- Open/close state: S05.

## 7. Implementation notes (2026-11-05)

- Windows locks through `LockFileEx`/`UnlockFileEx` loaded from `kernel32.dll` via `syscall`; no
  `golang.org/x/sys` dependency. Unix uses `syscall.Flock`. Other platforms report
  `coordinator.unsupported`.
- Byte 0 of a lock file is the locked byte; the holder JSON starts at byte 1, so a reader can skip
  the mandatory-locked byte on Windows. This mirrors the DocEngine reference.
- Same-process contention is covered by the tests: two handles in one process conflict, which is
  what makes the crossing-acquire test meaningful without a helper binary.
- Lock files are never deleted. Deleting one while another process has it open would reintroduce
  the race the OS lock avoids; the lock state ends with the process.
- The lock directory is `Layout.Coordinator()` (`<store>/.coordinator`) and lives inside the store,
  so the existing ignore rule covers it.
- `Acquire` returns a no-op release for an empty resource set; a command that locks nothing does not
  fail.
- `Shared` was added while implementing S05a: body writes hold the book shared and the block
  exclusively, close holds the book exclusively, so a repack waits for every in-flight write without
  serializing writers among themselves. Unix uses `LOCK_SH`, Windows `LockFileEx` without the
  exclusive flag. Shared holders write no holder metadata, because several of them would race on the
  file.

