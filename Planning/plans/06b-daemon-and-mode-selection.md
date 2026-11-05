# S06b — Implementation Plan: Daemon Backend and Mode Selection

Status: implemented (2026-11-05). Parent scope: [../06-concurrency-and-coordination.md](../06-concurrency-and-coordination.md).
Normative source: [../../PLAN.md](../../PLAN.md) §3.12, §6.

Second and last delivery slice of S06.

## 1. Decisions

| # | Question | Decision | Rationale |
|---|---|---|---|
| 1 | Transport | one AF_UNIX socket on every platform (owner decision 2026-11-05) | One code path with `net.Listen("unix")`; local-only; no Win32 named-pipe implementation. Recorded in `PLAN.md` §3.12 |
| 2 | Socket path | `concurrency.daemon.socket` if set, otherwise `<os.TempDir>/clabok-<sha256(storeRoot)[:8]>.sock` | Short enough for the AF_UNIX limit on Windows and unique per store |
| 3 | Protocol | one JSON line request, one JSON line response, then the connection holds the lock | A held connection is a natural lock lease; release is the close |
| 4 | Request | `op`, `version`, `command`, `pid`, `mode`, `timeoutMs`, resources | The holder text and the version handshake need no second round trip |
| 5 | Server lock table | all-or-nothing grant under one mutex, 20 ms polling until the deadline | No partial holds, no ordering-related deadlock, simple to reason about |
| 6 | Version handshake | the client sends its version; the server rejects a mismatch with both versions | `PLAN.md` §3.12 |
| 7 | Availability | the client fails with `coordinator.daemon-unavailable` when no daemon listens; it never starts one | `PLAN.md` §3.12 |
| 8 | Double start | a live socket is `coordinator.already-running`; a stale socket file is removed | A stale file after a crash must not block the next start |
| 9 | `serve` | foreground command on the same binary, listening line on stderr | `PLAN.md`: a mode of the same binary, not a second product |
| 10 | Mode selection | `concurrency.mode` chooses the backend in the command layer; the core still sees only `Coordinator` | `PLAN.md` §3.12 |
| 11 | Mode change | rejected with `config.open-books` while any book is open | `PLAN.md` §3.12 |
| 12 | Conformance | one semantics suite runs against both backends | Backend parity is an invariant |

## 2. Wire protocol

```json
{"op":"acquire","version":"0.2.0-dev","command":"book","pid":1234,"mode":"exclusive","timeoutMs":5000,"resources":[{"kind":"store"},{"kind":"book","book":"clabok"}]}
{"ok":true}
{"ok":false,"code":"coordinator.busy","message":"book:clabok is held by pid 99 (book); waited 5s"}
```

## 3. Files

```
internal/coordinator/daemon.go      socket default, wire types, client, server, lock table
internal/coordinator/command.go     serve command
internal/coordinator/daemon_test.go conformance suite for both backends
internal/lifecycle/command.go       mode selection
internal/store/state.go             OpenBookCount for the mode guard
internal/config/command.go          open-book guard on a mode change
cmd/pecia/main.go                   register serve
```

## 4. Acceptance

| S06 criterion | Evidence |
|---|---|
| 3 daemon backend with in-memory mutexes | conformance subtest |
| 4 identical semantics | shared/exclusive/busy/crossing tests run against both |
| 5 per-block concurrency, per-node serialization | shared/exclusive resource tests |
| 9 mode selection, open-book guard | config command test |
| 10 local-only transport | no TCP listener exists in the code |
| 11 version match | mismatch test |
| 12 no data difference | the backend only coordinates |

## 5. Out of scope

- Windows named pipes: superseded by the owner decision.
- Daemon autostart, service installation, remote transport.
- TUI and per-command lock choices: S14, S07, S08.

## 6. Implementation notes (2026-11-05)

- AF_UNIX on Windows needs Windows 10 1803 or newer. The socket path is kept short because the
  Windows AF_UNIX limit is about 108 bytes.
- The lock table grants all resources in one step, so there is no partial hold and therefore no
  ordering deadlock in the daemon. The file backend still sorts resources for the same reason.
- The server holds the lock until the client closes the connection; a client crash releases it
  without a stale-lock cleanup, matching the file backend.
- `Listen` probes the socket first: a successful dial means another daemon owns it; a failed dial
  means the file is stale and can be removed.
- The conformance suite caught a test-only bug: releasing the first successful acquire before both
  goroutines reported made the waiting acquire succeed. Both crossing tests now collect both
  results before releasing.
