# S13 Implementation Plan — Grammar Gate and Term List

Status: delivered (2026-11-06)
Scope: [../13-grammar-and-terms.md](../13-grammar-and-terms.md)
Normative source: [../../PLAN.md](../../PLAN.md) §3.11, §3.14, §3.7, §6.

## 0. Principles

- The check never writes; the gate runs inside `body.Service.write` (set/append/prepend) before the
  snapshot, so a refused write leaves the body unchanged.
- `block`, `warn` and `off` are the only policies. There is no silent third state.
- Terms and pending term requests live in the store; a pending request survives a restart.
- The suite is offline and deterministic: an `httptest` fake server stands in for LanguageTool; a
  real server is an explicit integration path.
- An unreachable server degrades the check to "no findings" and is reported once. It never fails a
  write by itself.

## 1. Delivery slices

| Slice | Content | Why |
|---|---|---|
| **S13a** | HTTP client, findings, policy config, the gate in `body.write`, `grammar check` | The gate and its reporting |
| **S13b** | `grammar apply` with selected findings, atomic write and one snapshot | The repair path |
| **S13c** | Terms store, pending requests, confirmation, suppression and the `terms` commands | The tool-owned vocabulary |
| **S13d** | `grammar serve`, `server-stop`, `server-info`, one shared server and the temp state | The server lifecycle |

## 2. Decisions

### D1 — Client

`grammar.endpoint` holds the full check URL; the default is
`http://127.0.0.1:8081/v2/check`. A check POSTs `text=<urlencoded>&language=en-US` with a 5-second
timeout and parses the LanguageTool `matches` array. `en-US` is fixed; the endpoint is the only
grammar config key (S02 owns the rest).

### D2 — Findings

A finding is `{index, block, line, column, offset, length, rule, message, replacements}`. Line and
column are 1-based; `offset`/`length` address the exact range in the checked text. `index` is 1-based
and stable only for the run that produced it. JSON is the default output; `--text` renders one line
per finding. Terms never appear in the model as findings.

### D3 — Policy and gate placement

`grammar.policy` is a new `clabok.json` key with strict validation: `block` (default) or `warn`.
There is deliberately no `off`: every prose write is checked, and only a server that cannot be
started degrades to a reported skip. The gate runs before the snapshot on every prose write path:
`body set|append|prepend`, `body replace`, `prose set`, `region set` (footer), `region notes` and
`layout note`. `block` returns `body.grammar-blocked` with the findings as details; `warn` writes and
returns the findings in the payload. `grammar apply` is the gate's own repair path: it re-checks the
result and reports the remaining findings.

### D4 — Dry run

A dry run still checks and reports (the check is read-only), but never writes. A `block` finding
fails the dry run exactly like a real write, so the preview matches the behaviour.

### D5 — Apply

`grammar apply --book <id> --block <id> (--all-safe | --finding N[,M...]) [--expect N] [--dry-run]`
re-checks the current body, selects `--all-safe` (only findings with exactly one replacement) or the
explicit 1-based indices, and applies the replacements right-to-left by offset, so earlier offsets
stay valid. One `SnapshotBody` and one `WriteAtomic` per run; `--expect` guards the number of applied
replacements (`grammar.expectation-failed`). The result is re-checked and the remaining findings are
reported; the repair path does not block a partial fix because every replacement is server-provided.
A finding index is only valid for the check that produced it; `apply` always re-checks first.

### D6 — Terms file

`<store>/terms.txt` is store-wide (one term per book set is not needed; the store is the project).
UTF-8, one term per line, blank lines ignored, no comments. Matching is case-insensitive; the first
written form is kept. The tool owns the file.

### D7 — Pending requests and confirmation

`<store>/pending-terms.json` holds `{id, action, term, reason, created}`. `terms set` and
`terms delete` append a pending entry and never touch `terms.txt`; the ID is derived from the entry
content. `terms pending` lists, `terms discard` removes. `terms commit --id <id> --confirm` is the
only writer: without `--confirm` it fails with `terms.confirmation-required`; with it, the list is
updated once and the entry removed. The reason is the recorded justification; there is no separate
audit log.

### D8 — Suppression

A finding is suppressed when its offending text or any of its replacements contains a term
case-insensitively. Suppression is applied after the HTTP check and before the gate decision, so a
suppressed finding behaves exactly like a clean result.

### D9 — Server lifecycle

`grammar serve` derives the base URL and port from the configured endpoint (local hosts only:
`127.0.0.1`, `localhost`, `::1`) and probes `GET <base>/v2/languages`. If a server answers, it
attaches and reports it; otherwise it resolves the jar and starts
`java -cp <jar> org.languagetool.server.HTTPServer --port <port> --allow-origin *` detached.

Jar resolution order: `--jar`, `CLABOK_LANGUAGETOOL_JAR`, `LANGUAGETOOL_JAR`, then the popular
`language_tool_python` caches (`~/.cache/language_tool_python`,
`$LOCALAPPDATA/language_tool_python`, `<temp>/language_tool_python`). Java must be on `PATH`
(`grammar.no-java` otherwise). The start waits up to 40 seconds for `/v2/languages`; an exclusive
lock (60 seconds) ensures exactly one start, and a second `serve` waits and attaches.

The state (`state.json`), lock and log live in `os.TempDir()/clabok-grammar` (override for tests:
`CLABOK_GRAMMAR_STATE_DIR`), never in the store. `server-info` reports the state and a live probe;
`server-stop` stops the recorded pid and removes the state. Without a jar and without a reachable
server, `serve` rejects with `grammar.no-jar`.

On demand: `Gate.Evaluate` and `grammar check` start the shared server when the endpoint is
unreachable and check once more. Only a start failure degrades to `grammar check skipped: <reason>`;
a prose write is never silently skipped.

Tests: the deterministic unit paths use an `httptest` fake. `TestLanguageToolLive` starts the real
server through `Serve` when Java and a jar are available, checks a known-wrong sentence, stops the
server and is skipped otherwise (and in `-short` mode).

### D10 — Commands

```
pecia grammar check       --book <id> --block <id> | --file <p> | --raw <text>
pecia grammar apply       --book <id> <block> (--all-safe | --finding <list>) [--expect <n>]
pecia grammar serve       [--jar <path>]
pecia grammar server-stop
pecia grammar server-info
pecia terms set           --term <t> --reason <r>
pecia terms delete        --term <t> --reason <r>
pecia terms pending
pecia terms commit        --id <id> --confirm
pecia terms discard       --id <id>
pecia terms list
```

Book-bound commands are open-only and dry-run aware. `terms` commands work on the store and need no
open book.

## 3. S13a tasks

1. `config`: add `grammar.policy` with `block`/`warn`, strict validation and the default `block`.
2. `internal/grammar`: the client, the finding model, the policy, the gate report and the on-demand
   server start.
3. The gate runs before the snapshot on every prose path: `body set|append|prepend` and
   `body replace`, plus `prose set`, `region set`, `region notes` and `layout note` in editorial.
   `block` rejects with `body.grammar-blocked` and the findings as details; `warn` reports them.
4. `grammar check` for a block, a file and raw text; the text output lists findings and the degraded
   state once.
5. Tests: the fake server matrix (clean, finding, block, warn, degraded, dry run), the editorial
   paths and the real-server integration test; term suppression is added in S13c.

## 4. S13b tasks

Delivered 2026-11-06.

1. The apply path writes through the lifecycle lock and `SnapshotBody`/`WriteAtomic` (one package, no
   body-package cycle) and re-checks the result.
2. `grammar apply` selection, right-to-left replacement, `--expect`, dry run.
3. Tests: only the selected findings change, the expectation guard, stale findings and atomicity.

## 5. S13c tasks

Delivered 2026-11-06.

1. `internal/grammar/terms.go`: the store-level term list and the pending store.
2. The `terms` commands with the confirmation gate.
3. Suppression in the gate via the term list.
4. Tests: no write on request, pending survives a restart, commit/discard, suppression on/off.

## 6. S13d tasks

Delivered 2026-11-06.

1. The temp state, lock and log; the endpoint probe; the detached start and the stop path.
2. `grammar serve|server-stop|server-info`; one shared server under concurrent starts.
3. Tests: two attaches to a fake endpoint, the lock, the state round-trip, the stop of a recorded
   process.

## 7. Acceptance mapping

| # | Criterion | Slice |
|---|---|---|
| 1 | block/warn/off matrix on a prose write | S13a |
| 2 | no server degrades once and applies the policy | S13a |
| 3 | apply changes only the selected findings, atomically | S13b |
| 4 | terms set/delete never write the list | S13c |
| 5 | commit without confirmation fails; with it, one write | S13c |
| 6 | a pending entry survives a restart | S13c |
| 7 | a term suppresses and its removal restores the finding | S13c |
| 8 | two checks share one server; server-info reports it | S13d |
| 9 | the temp dir holds state, lock and log; the store none | S13d |

## 8. Verification

- `gofmt -l .`, `go vet ./...`, `go build ./...`, `go test ./... -count=1`, then
  `.\pecia.exe sync --check` (must be clean).
- The suite stays offline; the fake server is the default test path.
