# S07b — Implementation Plan: Declarative Apply Plans

Status: implemented (2026-11-05). Parent scope: [../07-structure-and-book-settings.md](../07-structure-and-book-settings.md).
Normative source: [../../PLAN.md](../../PLAN.md) §3.7, §3.13 (`internal/apply`).

Second and last delivery slice of S07.

## 1. Decisions

| # | Question | Decision | Rationale |
|---|---|---|---|
| 1 | Plan format | `{"version":1,"steps":[...]}` with ops `add`, `rename`, `move`, `remove`, `setting` | One declarative surface over the same service logic; content ops arrive with S08 |
| 2 | Atomicity | one book lock, one validation, one undo package and one manifest write for the whole plan | `PLAN.md` §3.7: one rollback point |
| 3 | Pure steps | the service applies a step to an in-memory manifest and returns the receipt plus file effects; the commit writes once | No per-step snapshots or writes; failure leaves the book untouched |
| 4 | Public commands | `add/rename/move/remove/book set` are one-step plans through the same runner | No second implementation of structure logic |
| 5 | Placement fields | `parent` / `childOf` / `index` / `before` / `after`; `root` names the book root | Mirrors the command flags |
| 6 | Unknown fields | strict decoding | A typo in a plan must not be ignored |
| 7 | Empty plan | rejected with `apply.empty-plan` | An empty batch is a caller error, not a no-op |
| 8 | File input | `--file <path>`, `-` reads stdin | Tooling can pipe a plan |
| 9 | Dry run | parse, apply in memory, validate, report; nothing written and no undo package | Same contract as the other mutating commands |
| 10 | Validation failure | `book.invalid` reject with the issue list | A plan must never persist an invalid book |

## 2. Error codes

`apply.invalid-plan`, `apply.version`, `apply.empty-plan`, `apply.invalid-step`,
`apply.read-failed`, plus the `structure.*` codes and `book.invalid` from the service.

## 3. Files

```
internal/apply/apply.go       plan parse, command, payload
internal/apply/apply_test.go  parse matrix, batch execution, atomicity, CLI
internal/structure/structure.go  Step, Execute, pure step functions
internal/structure/command.go    MapError exported
cmd/pecia/main.go                register apply
```

## 4. Acceptance

| S07 criterion | Evidence |
|---|---|
| 9 atomic plan with one rollback point | batch test (one undo), failure test (no write, no undo) |
| 9 validation before write | service validation, `book.invalid` reject |
| 9 delegates to the same services | single commands are one-step plans |
| 10 dry run | byte comparison and undo count |
| 12 resulting order in JSON | receipt payload per step |

## 5. Out of scope

- Body/content steps: S08 adds a `body` op on top of the same runner.
- Library-wide plans: S11.
- Region and status steps: S12.

## 6. Implementation notes (2026-11-05)

- The refactor moved the mutation logic out of the public methods into pure step functions, so a
  plan and a single command share one code path. The public methods are now thin wrappers.
- The plan runs against the loaded manifest; a failing step returns before validation, so no
  snapshot is taken and nothing is written.
- The layering test now lists only the pure packages; `apply` carries a command surface like
  `structure` and therefore imports the CLI by design.
- Content steps are deliberately absent until S08 defines the body write rules and the grammar gate
  call site.
