# S08a — Implementation Plan: Body Authoring and Prose Discipline

Status: implemented (2026-11-05). Parent scope: [../08-body-authoring-and-prose.md](../08-body-authoring-and-prose.md).
Normative source: [../../PLAN.md](../../PLAN.md) §3 (3, 5, 6), §3.7 (body writes), §2.5.

First delivery slice of S08. S08b adds the temp area, `flush` and `find`.

## 1. Decisions

| # | Question | Decision | Rationale |
|---|---|---|---|
| 1 | Parallelism | a body write holds the book **shared** and the affected block **exclusively**, in that order | Different blocks of one book are written concurrently; only `close` takes the book exclusively |
| 2 | Reads | no lock; the store writes atomically | A reader always sees a complete body |
| 3 | Undo | a body write snapshots only its block (`KindBody`) | No whole-book copy per write; parallel writers do not interfere |
| 4 | Block 0 | creates a block through `structure.AddBlock` (the parent kind does not decide: a chapter can hold blocks), then writes the body | The block rule is body-driven, not structure-driven |
| 5 | Creation undo | the structure add's package covers the pre-add state; the body write adds none | One command, one undo package |
| 6 | Heading rule | lines with 1–3 leading `#` are rejected outside fenced code; `####`+ and four-space indentation are text | `PLAN.md` §3 (6); never rewrite, only validate |
| 7 | Gate | the rejection is an informing reject (`body.forbidden-heading`) with the line numbers | "A gate that blocks is an informing reject" |
| 8 | Encoding | UTF-8 only, BOM rejected, CRLF/CR normalized to LF before writing | Deterministic bodies; the store is LF |
| 9 | Append/prepend | a separator newline is inserted only when the pieces would otherwise touch | Predictable concatenation |
| 10 | Replace | literal pairs in one left-to-right pass (longest first); any regex pair switches to sequential regex runs; `--expect` counts the total | No cascade for literals; regex users get standard `$1` expansion |
| 11 | Block list | depth is a number, taken from the node depth | The reference defect class (`body list` JSON) |
| 12 | CLI flag | the body content flag is `--body`; `--text` is the global human-output flag | A flag cannot be defined twice |

## 2. Error codes

`body.unknown-block`, `body.parent-required`, `body.forbidden-heading`,
`body.expectation-failed`, `body.invalid-encoding`, `body.invalid-range`, `body.invalid-pair`,
`body.no-pairs`, `body.invalid-regex`, `body.read-failed`, `body.write-failed`.

## 3. Files

```
internal/body/body.go        Service, prose check, encoding, write path, replace
internal/body/command.go     body get|set|append|prepend|replace|list
internal/body/*_test.go      service, parallel-write and CLI tests
internal/rollback/rollback.go  KindBody, unique package names
internal/lifecycle/lifecycle.go  SnapshotBody, LockBlock, body restore, close cleanup
internal/structure/structure.go  AddBlock
```

## 4. Acceptance

| S08 criterion | Evidence |
|---|---|
| 1 bodies belong to blocks | block 0 creates a block; nodes are untouched |
| 2 body commands | get/set/append/prepend/replace/list tests |
| 3 block 0 rule | create test, parent-required test, unknown block test |
| 4 prose discipline | heading matrix, fenced code, four-space indentation |
| 5 heading gate | reject test, file unchanged |
| 10 encoding | BOM rejected, CRLF normalized |
| 11 undo and locks | body packages counted, parallel writes test |
| 12 list JSON | depth is a number |

## 5. Out of scope

- The grammar gate call site: S13 attaches to the same write path.
- Temp area and `find`: S08b.
- Notes and prose fields: S12.

## 6. Implementation notes (2026-11-05)

- `--text` is reserved by the global output flag; the body content flag is `--body`. The first test
  run caught the collision at startup.
- A write to an unplaced block was initially accepted because the write path only read the file;
  the locked manifest now verifies the placement before anything is written.
- Rollback package names gained a random segment. Two parallel writers can snapshot within the same
  clock tick, and the file name must stay unique without a central counter.
- The parallel test writes three blocks through two services and asserts three body packages, so the
  shared-book/exclusive-block path is exercised, not just described.
