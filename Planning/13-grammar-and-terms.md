# S13 — Grammar Gate and Term List

Status: draft, not planned.
Phase: 7. Depends on: S02 (grammar endpoint), S08 (prose-writing commands, temp). Blocks: none.
PLAN refs: §3.14 (terms and grammar), §3.7 (LanguageTool daemon state stays in the OS temp
directory), §3.11 (grammar endpoint), §6 (grammar gate on prose writes, confirmation flow).

## 0. Current state and pre-planned decisions (2026-11-05)

- The single prose-write path is `body.Service.write` (lock, normalize, `prose.CheckProse`,
  `SnapshotBody`, `WriteAtomic`); the grammar gate hooks in before the snapshot.
- Body gate failures are rejects: `body.forbidden-heading`, `body.expectation-failed` through
  `body.MapError`.
- `config.Values.GrammarEndpoint` exists (default `""` = local LanguageTool default); there is no
  policy key yet.
- `store.Layout.Terms()` is `<store>/terms.txt`; no code writes it.
- The coordination daemon (`pecia serve`, AF_UNIX) is unrelated to the grammar server.
- `apply` has no body step today; if it gains one, the gate must cover it.

Pre-planned decisions:

- Add `grammar.policy` (`block` default, `warn`, `off`) to `clabok.json` with strict validation.
- Findings `{block, line, column, rule, message, replacements}`; the block policy returns a reject
  with the findings as details.
- Terms: requests create pending entries; `terms commit` requires explicit confirmation; the tool
  never writes `terms.txt` directly.
- Tests use a fake HTTP server; a missing Java/LanguageTool wrapper degrades to a warning, not a
  failure.
- The server lifecycle is opt-in; the default is the local server, and the state, lock and log live
  in the OS temp directory.

## 1. Goal

Gate prose writes with a grammar check whose policy is explicit, and make the term list a
tool-owned, confirmation-gated artifact so that no prose write and no term change happens silently.

## 2. In scope

| # | Requirement | PLAN |
|---|---|---|
| 1 | LanguageTool client: HTTP check over the configured endpoint (S02); default is the local daemon in the OS temp directory | §3.11, §3.14 |
| 2 | Findings model: block ID, line, column, rule, message, replacements; JSON by default | §3.10 |
| 3 | Gate policy on prose-writing commands: `block` (default, refuse a finding), `warn` (write and report), `off` (skip); a missing Java/wrapper/server degrades to a warning, never a hard failure | §3.14 |
| 4 | Applying selected findings: a batch replace with an expected count, atomic, one rollback package | §3.14, §6 |
| 5 | Term list: tool-owned and lives in the store; `set` and `delete` are requests that create a pending entry, never a write; commit requires explicit confirmation; `pending`, `discard` and batch commit exist | §3.14, §6 |
| 6 | Terms suppress invented names and domain compounds in findings; the term list never changes a body by itself | §3.14 |
| 7 | Server lifecycle: `serve` starts or attaches to the one shared server, `server-stop`, `server-info`; concurrent checks share one server; idle timeout is configurable | §3.11, §3.14 |
| 8 | Daemon state (state file, lock, log) stays in the OS temp directory and is never part of the store | §3.7 |
| 9 | The gate runs inside the body-write path defined by S08; it never writes by itself | §3.14 |
| 10 | Offline and failure behaviour is explicit and stable: an unreachable server yields a warning and the configured policy applies to a finding-free result | §3.14 |

## 3. Out of scope

- Body read/write and replace mechanics: S08 (this scope plugs into its write gate).
- The Unity editor grammar bridge: not carried over; CLABOK has no Unity editor.
- Terms UI beyond the commands: S14 may surface them, it does not own them.
- Grammar configuration keys beyond the endpoint: S02.
- Any change to docengine or its LanguageTool wrappers: read-only reference.

## 4. Deliverables

- `internal/grammar`: client, findings, policy enforcement, finding application, server lifecycle
  commands.
- `internal/grammar` terms store (or the owning package for `terms.txt` in the store): pending
  requests, confirmation, commit, discard, lookups.
- A fake server used by tests so the suite is offline and deterministic.
- Commands: grammar check (book, block, file or raw text), grammar apply, terms set/delete/pending/
  commit/discard/list.
- Tests: gate matrix (block/warn/off), server-absent matrix, apply atomicity, pending/commit flow,
  term suppression, concurrent checks sharing one server.
- This scope's implementation plan under `Planning/plans/`.

## 5. Acceptance criteria

1. With policy `block`, a prose write that produces a finding is refused and the body is unchanged;
   with `warn` it is written and the findings are reported on stderr/stdout per the output contract;
   with `off` no check runs.
2. With no server reachable, the configured policy still applies to a finding-free result and the
   command reports the degraded check exactly once (no repeated warnings per finding).
3. Applying selected findings changes only the chosen findings, atomically, with a rollback package.
4. `terms set` and `terms delete` never modify `terms.txt`; they return a pending ID and a reason.
5. `terms commit` without the explicit confirmation fails; with it, the term list is updated once and
   the pending entry is removed.
6. A pending entry survives a process restart and can be recovered.
7. A term suppresses its finding in a subsequent check; removing it restores the finding.
8. Two concurrent checks start or share exactly one server process; `server-info` reports it.
9. The OS temp directory holds the server state, lock and log; the store contains none of them.

## 6. Design decisions for this scope's planning session

- Whether grammar is part of the first delivery at all, or a later opt-in module (it is not required
  for the M5 milestone).
- Finding identity: how a finding is addressed for `apply` (stable ID per run, or line/column/rule).
- Replacement policy: only unambiguous single-suggestion findings are auto-applied by default.
- Terms file format, case sensitivity, deduplication and the confirmation record (who, when, reason).
- Server start semantics: whether `serve` is required before a check in `daemon`-like setups or the
  check starts a short-lived local server; `PLAN.md` requires one shared server, not a specific
  start path.
- Timeouts and retry policy for slow servers, and whether a timeout blocks a write under `block` or
  degrades.
- Whether `grammar check --file`/`--text` (non-book prose) is in scope; it is useful for the tool's
  own documentation and cheap to add.
- How the gate interacts with `--dry-run` (no check, or check and report without writing).

## 7. Risks and constraints

- A grammar server is an external process. The test suite must not depend on a network or on Java;
  use a fake server and an explicit integration test that is skipped when unavailable.
- The gate must never become a reason a valid write is lost silently. `block` is a refusal with an
  error code, `warn` is a write with a report; there is no third, silent state.
- Term changes are the only path that modifies the suppression list. Direct file edits are a defect
  even if the format is plain text.
