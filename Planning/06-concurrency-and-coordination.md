# S06 — Concurrency and Coordination

Status: draft, not planned.
Phase: 3. Depends on: S04 (store, atomic writes). Blocks: S05, S07, S08, S12, S14.
PLAN refs: §3.12 (complete), §3.5 (close coordination), §3.7 (stale and concurrent changes),
§6 (identical backend semantics, per-block concurrency, stable lock order).

## 1. Goal

Provide one coordinator contract with two interchangeable backends — file locks and an in-memory
`pecia serve` daemon — so that concurrent authoring in two processes is safe, per-block edits run in
parallel, structural changes serialize per node, and no backend changes book data.

## 2. In scope

| # | Requirement | PLAN |
|---|---|---|
| 1 | One coordinator interface consumed by book logic; the core never depends on a backend | §3.12 |
| 2 | `file` backend (default): lock files, atomic writes, stale detection | §3.12 |
| 3 | `daemon` backend: in-memory mutexes in one `pecia serve` process, same binary, a mode not a second product | §3.12 |
| 4 | Identical semantics for both: one writer per locked resource, consistent reader snapshots, no lost writes, stale-session detection | §3.12, §6 |
| 5 | Granularity: body edits lock one block ID; manifest changes lock the affected structural node or the book's structure lock (add, remove, rename, move, reparent, reorder) | §3.12 |
| 6 | Lock ordering: multi-lock operations acquire book -> node -> block with timeouts, so crossing moves cannot deadlock | §3.12 |
| 7 | Reads use an atomic snapshot and need no write lock; a reader never sees a half-written block | §3.12 |
| 8 | Close coordination: a close waits for or rejects in-flight block edits and never repacks a body being written | §3.12 |
| 9 | Mode selection from S02 (`concurrency.mode`), CLI flags override; switching the mode while a book is open is rejected | §3.12 |
| 10 | Daemon transport is local only: named pipe on Windows, Unix socket elsewhere; no TCP listener | §3.12 |
| 11 | The CLI expects a running daemon in `daemon` mode and fails with a clear error when it is absent; it never auto-starts one | §3.12 |
| 12 | CLI and daemon must run the same version; a mismatch is a clear error | §3.12 |
| 13 | The backend never changes book data, IDs, rendering or output | §3.12 |
| 14 | A shared conformance suite runs the same semantics test against both backends | §6 |

## 3. Out of scope

- Open/close state machine and lifecycle policy: S05 (it consumes the coordinator).
- Which command acquires which lock: declared by S07, S08 and S12; this scope provides the primitive
  and the ordering helper.
- Remote or network coordination: explicitly excluded by `PLAN.md`.
- Daemon management UI or service installation; `serve` is a process mode, nothing more.
- TUI concurrency behaviour: S14.

## 4. Deliverables

- `internal/coordinator`: interface, resource identity (book, node, block), lock ordering helper,
  timeout and stale-session errors, reader snapshot contract.
- `internal/coordinator/file`: lock files, stale detection, atomic write integration.
- `internal/coordinator/daemon`: client plus server (`pecia serve`) and the local transport
  (`_windows.go`, `_unix.go` with build tags).
- Conformance test suite parameterized by backend.
- Multi-process integration tests using tiny helper binaries or re-exec of the test binary.
- Version handshake between CLI and daemon.
- This scope's implementation plan under `Planning/plans/`.

## 5. Acceptance criteria

1. Two processes edit two different blocks of the same structural node concurrently; both writes land.
2. Two processes write the same block; one wins, the other fails with a stable conflict or stale
   code, and no write is lost or merged.
3. Two crossing moves (A into B's subtree while B moves into A's) cannot deadlock: the test terminates
   within the configured timeout and one operation is rejected.
4. A reader during a concurrent write never observes a partial body (test reads in a tight loop while
   a writer rewrites a large body).
5. A stale lock from a dead process is reclaimed after the configured timeout, under both backends.
6. Close during an in-flight body write either waits and repacks the final state or rejects the close;
   it never repacks a partial body.
7. Switching `concurrency.mode` while a book is open is refused.
8. In `daemon` mode without a running server, a mutating command fails with a clear error and does
   not auto-start anything.
9. A CLI/daemon version mismatch is refused with the two versions in the message.
10. The conformance suite is green against both backends, including on Windows.
11. `go test -race` is clean for the whole repository.

## 6. Design decisions for this scope's planning session

- Lock file content: owner identity, session token, lease deadline, and the staleness criterion
  (heartbeat versus timeout versus pid liveness).
- Timeout defaults and whether they are configurable (S02 currently lists only the mode and the
  socket path; adding a timeout is an owner decision if it should be user-visible).
- Named pipe name and Unix socket path defaults, and their derivation from the project path.
- Daemon lifecycle: start, readiness, idle timeout, shutdown, and what happens to open books when the
  daemon stops (the state file is the truth; decide the report).
- Whether the daemon is stateless apart from mutexes and book snapshots, or caches books.
- Snapshot semantics for reads: whole manifest, per file, or per book folder.
- How the coordinator interface exposes a multi-lock operation (a callback that receives all locks,
  or explicit acquire/release with ordering enforced by the helper).
- Whether lock files live in the store or the temp area (`PLAN.md` does not fix it; they are transient
  and must not be committed, which argues for the temp area).

## 7. Risks and constraints

- File locking on Windows and Unix differs. Test on both; do not assume POSIX semantics.
- A daemon that caches state becomes a second source of truth. Keep the book state in the store and
  limit the daemon to coordination.
- Stale detection is the failure mode that produces lost writes. Test with a real killed process, not
  a simulated flag.
- Backend parity is an invariant. Any behaviour that only one backend can produce is a bug.
