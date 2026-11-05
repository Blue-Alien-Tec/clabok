# S08b — Implementation Plan: Temp Area, Flush and Find

Status: implemented (2026-11-05). Parent scope: [../08-body-authoring-and-prose.md](../08-body-authoring-and-prose.md).
Normative source: [../../PLAN.md](../../PLAN.md) §3.3, §3.13 (`internal/search`).

Second and last delivery slice of S08.

## 1. Decisions

| # | Question | Decision | Rationale |
|---|---|---|---|
| 1 | Temp root | `temp.root` from the config, per book `<tempRoot>/<bookId>/` | `PLAN.md` §3.3; resolved by S02 |
| 2 | Flush scope | `--book` or `--all`, never both | An explicit scope prevents an accidental sweep |
| 3 | Flush write | listing by default; deletion only with `--write` | `PLAN.md`: delete only after explicit confirmation |
| 4 | Flush safety | every path must stay inside the temp root; the store is never touched | A flush is not a cleanup of durable data |
| 5 | Find fields | `body` and `title`; notes and prose follow with S12 | The model has no notes/prose yet |
| 6 | Find options | case, whole word, context lines, count vs occurrences | Tooling needs both the places and the totals |
| 7 | Find reads | no lock | Atomic files; a search does not block writers |
| 8 | Count output | `--count` omits the match list but keeps both counters | Stable machine output |

## 2. Error codes

`temp.invalid-root`, `temp.scope-required`, `temp.invalid-path`, `temp.read-failed`,
`temp.write-failed`, `search.empty-query`, `search.unknown-field`, `search.read-failed`.

## 3. Files

```
internal/temp/temp.go        Service: list and delete under the temp root
internal/temp/command.go     flush
internal/search/search.go    Service: find over titles and bodies
internal/search/command.go   find
```

## 4. Acceptance

| S08 criterion | Evidence |
|---|---|
| 7 temp area and flush | list/delete tests, scope and path guards |
| 9 find | field, case, word, context and count tests |
| 10 no store writes | flush paths are checked against the temp root |

## 5. Out of scope

- Note and prose fields: S12.
- Grammar findings: S13.
- Scheduled cleanup or size limits: not in `PLAN.md`.

## 6. Implementation notes (2026-11-05)

- `flush` reports `dryRun: true` unless `--write` is given; the global `--dry-run` also suppresses
  deletion, so the receipt contract stays uniform.
- A missing temp root is an empty listing, not an error.
- `find` lowercases for the case-insensitive count; reported text stays original.
