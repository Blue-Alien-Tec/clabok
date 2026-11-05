# S01 — Implementation Plan: Foundation and CLI Contract

Status: planned (2026-11-05). Scope: [../01-foundation-and-cli-contract.md](../01-foundation-and-cli-contract.md).
Normative source: [../../PLAN.md](../../PLAN.md).

## 1. Decisions

All items below are implementation design. The owner guidance applied: optimal performance and
robustness, no error for a normal outcome, a blocking gate is an informing reject, complexity only
where it buys robustness. Reference basis: [../reference-docengine.md](../reference-docengine.md).

| # | Question | Decision | Rationale |
|---|---|---|---|
| 1 | JSON envelope | `{"schema":1,"ok":true,"command":"...","data":{...}}`, errors as `{"schema":1,"ok":false,"command":"...","error":{"code","message","details"}}` | One stable outer shape; `data` keeps payloads evolvable without touching the envelope |
| 2 | Schema version | one integer for the whole CLI contract, bumped only on a breaking change | Per-command versions multiply states nobody can test |
| 3 | Exit codes | `0` ok, `1` failure, `2` usage, `3` rejected | A gate reject is a normal outcome, not a failure; tooling can branch without parsing messages |
| 4 | Reject | `ok:false`, `error.code` in the `reject.*` namespace, exit 3, nothing written, details carry the findings | "Informing reject": the caller gets the complete reason and the book is untouched |
| 5 | Error codes | `<domain>.<condition>`, lower-kebab (`usage.unknown-flag`, `config.unknown-key`, `model.depth-exceeded`); each package owns its namespace; every error carries a `hint` (one actionable sentence, data not prose) | No central mega-list, no import cycles, stable strings; DocEngine's hint is useful, its message-derived codes are not |
| 6 | Output streams | data on stdout, diagnostics on stderr, never mixed | Machine parsing and human reading in one process |
| 7 | JSON formatting | two-space indent, HTML escaping off, trailing newline | Deterministic, golden-testable, readable |
| 8 | `--text` | `Result.Text` if set, otherwise pretty-printed JSON as the last resort | `--text` never fails a command that otherwise succeeded |
| 9 | `--dry-run` | central flag; a mutating command must return a payload implementing `DryRunResult() bool`, otherwise `internal.dry-run-missing` | Enforceable at the dispatcher, testable with a dummy command |
| 10 | Global flags | known globals may appear before or after the command; a stdlib `FlagSet` pre-parse handles the leading block, the command's `FlagSet` re-registers the same variables | Correct value handling, no custom argument scanner, `--` works |
| 11 | Command lookup | flat registry keyed by `"group sub"`; longest prefix match of the leading positionals | Two levels are enough for the whole surface, no tree walking |
| 12 | Unknown command | `usage.unknown-command`, exit 2, `details.suggestion` from a cheap edit-distance match | Normal user error, not an exception |
| 13 | Panics | recovered at the dispatcher, panic and stack to stderr, `internal.panic` envelope, exit 1 | Preserves the contract for tooling, still visible as a defect |
| 14 | `help` | built-in command, JSON default, `--text` renders usage | Consistency with the JSON-first rule |
| 15 | `version` | built-in, version injected via `-ldflags -X .../internal/buildinfo.Version` | One source of truth, no generated files |
| 16 | Dependencies | standard library only | Static binary, `CGO_ENABLED=0`, no owner approval needed |

## 2. Contract

Envelope:

```json
{"schema": 1, "ok": true, "command": "version", "data": {"product": "CLABOK", "cli": "pecia", "version": "0.1.0-dev"}}
```

```json
{"schema": 1, "ok": false, "command": "config.get", "error": {"code": "config.unknown-key", "message": "...", "hint": "run pecia config show for the effective values", "details": {"known": ["store.root"]}}}
```

| Kind | Exit | Produced by |
|---|---|---|
| success | 0 | any command, including no-ops |
| failure | 1 | real error: unreadable store, corrupt container, failed write |
| usage | 2 | unknown command or flag, missing or invalid argument, conflicting global flags |
| reject | 3 | a gate refused the change and explained why; nothing was written |

## 3. Packages and files

```
cmd/pecia/main.go            wiring only: registry, config.Register, cli.Run, os.Exit
internal/buildinfo/          Product, CLI, Version (ldflags target)
internal/cli/errors.go       Error, Kind, Fail/Usage/Reject, AsError
internal/cli/globals.go      Globals, Env, FlagSet, leading and interspersed parsing
internal/cli/registry.go     Command, Registry, Add, Get, List, Suggest
internal/cli/dispatch.go     Run, panic recovery, dry-run enforcement, output boundary
internal/cli/output.go       envelope, JSON writer, text writer, Texter, exit codes
internal/cli/builtin.go      help and version
internal/cli/layering_test.go
```

Package rule: `internal/cli` provides the contract and never imports a domain package. Domain
packages register themselves through `cli.Command`.

## 4. Tasks

1. `buildinfo` constants and the ldflags-readable `Version` variable.
2. `cli.Error` with kinds and the `ErrorObject` projection; unit tests for the mapping.
3. `Registry` with duplicate-detection, sorted listing and suggestion; unit tests.
4. `Globals` + `Env` + `FlagSet`/`ParseFlags`; tests for before/after placement, `--flag=value`,
  `--`, unknown flag.
5. `Run`: pre-parse, command resolution, dispatch, panic recovery, exit-code mapping; tests.
6. Output: envelope, pretty JSON, text fallback; golden tests.
7. Dry-run enforcement; test with a registered dummy mutating command.
8. Built-in `version` and `help`.
9. Layering test: parse every `internal/**` package and fail if a domain package imports
  `internal/cli` or `cmd/`.
10. Rewrite `cmd/pecia/main.go` to wiring only.
11. `gofmt`, `go vet`, `go build`, `go test`, `go test -race`.

## 5. Test plan

- `version` JSON and `--text`.
- Envelope always carries `schema`; failed responses carry no `data`.
- Unknown command: exit 2, suggestion present.
- Unknown flag: exit 2, stable code, empty stdout.
- Global flag before and after the command; `--project=X` and `--project X`.
- `--` terminator passes the rest positionally.
- `--text` with and without `Result.Text`.
- Mutating command with `--dry-run` and a non-dry-run payload fails with `internal.dry-run-missing`.
- Panic in a handler: exit 1, `internal.panic`, stack on stderr, valid JSON on stdout.
- Help: exit 0, command list sorted, `--text` usage.
- Layering: no `internal/model`, `internal/store`, `internal/render`, `internal/refs` imports of
  `internal/cli` (list grows with the scopes).

## 6. Acceptance mapping

| Scope criterion | Evidence |
|---|---|
| 1 version JSON/text | builtin test |
| 2 unknown command/flag, exit 2, stable object | dispatch test |
| 3 central `--text`/`--dry-run` | globals + dry-run tests |
| 4 no stream mixing | output test asserts both writers |
| 5 toolchain green | `scripts/check.ps1` run |
| 6 dependency direction | `layering_test.go` |
| 7 one registration point | registry test with a dummy group |

## 7. Deferred

- A release build step and ldflags wiring in CI: not needed before S11; the variable works without
  it. There is no release stamping in this project; the version ladder is policy in
  [../README.md](../README.md) §4.1.
- Per-command JSON schemas on disk: the Go structs are the schema until tooling needs files.
- A `--json` flag: contradicts the JSON-default rule and is not introduced.

## 8. Implementation notes (2026-11-05)

- Command flags are parsed interspersed: flags work before, between and after the positionals, and
  a handler receives the positionals only. The leading pre-parse still stops at the command name so
  that command-specific flags are not mistaken for globals.
- The error object carries `hint`; `--text` prints it below the message.
- `--text` falls back to pretty JSON when a payload has no text form. A successful command never
  fails because the text form is missing.
- Global flags gained `--store-root`, `--temp-root` and `--docs-root` with S02, which makes the
  `PLAN.md` §3.11 precedence rule real.
- A flag between a group and its subcommand (`pecia config --text show`) resolves to the group and
  reports `usage.missing-subcommand`; put flags before the command or after the subcommand.
- `help` and `version` are registered by `cli.NewRegistry`; other command groups register
  themselves from `cmd/pecia`.
- `scripts/check.ps1` is blocked by the local execution policy in this environment; the same four
  commands were run directly. `go test -race` needs cgo and a C toolchain on Windows; the Linux CI
  job covers it.
