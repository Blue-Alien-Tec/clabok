# S01 — Foundation and CLI Contract

Status: implemented (2026-11-05). Plan: [plans/01-foundation-and-cli-contract.md](plans/01-foundation-and-cli-contract.md).
Phase: 1. Depends on: none. Blocks: every other scope.
PLAN refs: §3.8 (naming), §3.9 (project boundary), §3.10 (language, CLI contract), §3.13 (code
organization), §6 (acyclic dependencies, machine-readable output).

## 1. Goal

Create the Go project skeleton, the command dispatch and the output contract that every other scope
implements against, so that no later scope invents its own envelope, error shape, exit code or
package boundary.

## 2. In scope

| # | Requirement | PLAN |
|---|---|---|
| 1 | Go module at `F:\UnityProjects\CLABOK`, `CGO_ENABLED=0`, one static binary `pecia`; no .NET, Node or Python runtime | §3.10 |
| 2 | Package skeleton: `cmd/pecia` (flags, wiring, exit codes only) and `internal/cli` (dispatch, text/JSON output). No business logic in the entry point | §3.13 |
| 3 | Package responsibility rule: a package is named after what it provides; no `utils`, `helpers`, `common`, `models` or `types` catch-alls; dependency direction `cmd` -> `cli` -> application services -> domain/store, and the domain never imports the CLI | §3.13 |
| 4 | JSON is the canonical default output; `--text` renders a human-readable form. Data to stdout, diagnostics to stderr. No prompts, colors, paging or emoji | §3.10 |
| 5 | Stable error object (code, message, and the failing command), stable exit codes, and one documented table for both | §3.10 |
| 6 | `--dry-run` for every mutating command: the flag is parsed centrally and renders a receipt instead of writing. Per-command receipt content is the owning scope's business, the mechanism is this scope's | §3.10 |
| 7 | Versioned JSON schema: every response carries its schema version; the version and the product/CLI names come from one place | §3.10, §3.8 |
| 8 | Command surface skeleton: command groups (`book`, `body`, `config`, `library`, ...), global flags (`--project`, `--config`, `--text`, `--dry-run`), help/usage, `version`; groups stay empty until their scope lands | §3.10 |
| 9 | Test and CI baseline: table-driven unit tests, golden JSON fixtures, a `testdata/` convention, `gofmt` + `go vet` + `go build` + `go test` + `go test -race` on Linux and Windows | §6 |
| 10 | `doc.go` per package stating the package responsibility in one paragraph | §3.13 |

## 3. Out of scope

- Config file schema, discovery, precedence and `config` command behaviour: S02.
- Domain types and validators: S03.
- Any real book, container, lock, render or grammar behaviour: the owning scopes.
- TUI: S14. This scope only guarantees that the CLI is complete without it.
- Dependency tooling beyond an optional review aid (`depguard`); no gate that would block a build.

## 4. Deliverables

- `cmd/pecia/main.go` reduced to flag parsing, wiring and exit codes.
- `internal/cli`: dispatcher, output writer (JSON/text), error type, exit-code mapping, dry-run
  plumbing, help rendering.
- `internal/version` (or an equivalent single source) with the injected build version.
- `doc.go` files and the package dependency note for `PLAN.md` §3.13.
- A golden-test harness usable by every later scope.
- Updated `scripts/check.ps1`, `scripts/check.sh` and `release-ci.yml` if the baseline needs them.
- This scope's implementation plan under `Planning/plans/`.

## 5. Acceptance criteria

1. `pecia version` emits JSON with the schema version; `pecia version --text` emits the human form.
2. An unknown command and an unknown flag each fail with exit code 2 and a stable JSON error object
   on stderr; stdout stays empty of data.
3. Every command in the skeleton accepts `--text` and `--dry-run` without a per-command
  implementation, proving the flags are central.
4. A JSON response never mixes diagnostics into stdout (test asserts the streams separately).
5. `go vet`, `go build`, `go test` and `go test -race` are green on Linux and Windows; `gofmt -l .`
   is empty.
6. The dependency direction is documented and checkable: a test or review note shows that no
   `internal/` package imports `cmd/` or `internal/cli`, and that the domain packages import neither.
7. A new command group can be added by touching one registration point, proven by a test that
   registers a dummy group.

## 6. Design decisions for this scope's planning session

- JSON envelope shape: flat `{ok, command, ...}` versus a nested `{schema, result}`. Decide once;
  changing it later breaks every consumer.
- Schema versioning policy: integer per breaking change, semver, or one version per command group.
- Exit-code table: which codes exist beyond 0 (success), 1 (failure) and 2 (usage), and what a
  partial success means. `PLAN.md` requires stability, not a specific set.
- Error-code namespace and naming (`usage.unknown-flag`, `book.not-open`, ...), including the rule
  for adding codes without collisions across scopes.
- `--text` granularity: global flag versus per command; how a command that only makes sense as JSON
  (receipts) behaves under `--text`.
- Help output: text-only, or also JSON for tooling.
- Whether `--dry-run` receipts are a shared type or a per-scope shape with a shared envelope.
- How the version is injected (`-ldflags`) and how CI stamps it.

## 7. Risks and constraints

- The envelope and the exit codes are the hardest thing to change later. Spend the decision on them
  first and keep them minimal.
- The package list in `PLAN.md` §3.13 is a starting point, not a contract. Do not create empty
  packages for scopes that have not started; a package appears with its responsibility.
- `pecia.exe` is a build artifact in the working tree; it is ignored by `.gitignore` and must not be
  committed.
