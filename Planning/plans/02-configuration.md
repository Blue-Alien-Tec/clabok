# S02 — Implementation Plan: Configuration

Status: planned (2026-11-05). Scope: [../02-configuration.md](../02-configuration.md).
Normative source: [../../PLAN.md](../../PLAN.md) §3.11.

## 1. Decisions

| # | Question | Decision | Rationale |
|---|---|---|---|
| 1 | File shape | nested JSON objects, `version` first, sections omitted when empty | Strict decoding, readable diffs, no flat-key noise |
| 2 | Absence model | every leaf is a pointer (`*string`, `*int`); absent = default, explicit zero = zero | `unset` must restore the default, not write a zero |
| 3 | Missing file | not an error: effective values are the built-in defaults; `file` is `""`; `set` creates the file with `version: 1` | A fresh project must be usable without a ceremony command; errors only for broken input |
| 4 | Explicit `--config` that does not exist | `config.not-found` for read commands, allowed for `config set` | An explicit path that is missing is a user error, except where the user asks to write |
| 5 | Discovery | `--config`, then `--project` start dir, then upward from cwd to the filesystem root | Exactly `PLAN.md` §3.11, no global config |
| 6 | Path resolution | store/temp/docs roots are resolved against the config file directory (or the project dir when no file exists) and reported absolute | One rule for `--project` from any subdirectory |
| 7 | Key surface | dotted leaf paths plus the `asciiui.types.<name>` prefix | `show`/`get`/`set`/`unset` stay one implementation |
| 8 | `get` payload | `{key, value, source}` with `source` = `file` or `default`; path values are absolute | Debuggable without reading the file |
| 9 | `show` payload | `{file, projectDir, values}` with effective values only | Matches "effective values" in `PLAN.md` |
| 10 | `set` value syntax | JSON literal first, plain text for strings; one scalar per call | No shell quoting traps, type-safe per key |
| 11 | `unset` of a default | success, `changed: false`, no write | A no-op is a no-op |
| 12 | Validation | aggregate all issues, never stop at the first | One call tells the user everything |
| 13 | Strictness | unknown keys rejected, duplicate JSON keys rejected, trailing data rejected, BOM tolerated | Tolerant input, strict structure |
| 14 | Write | temp sibling + rename in the same directory, 0644, trailing newline, validate before replace | No partially written config |
| 15 | Dry run | `set`/`unset` return `{dryRun, key, from, to, changed}` and write nothing | Uniform receipt contract from S01 |
| 16 | `config init` | not added | `PLAN.md` enumerates the commands; implicit creation on `set` covers the need |

## 2. File shape

```json
{
  "version": 1,
  "store": { "root": "_CLABOK" },
  "temp": { "root": ".temp" },
  "docs": { "root": "Docs" },
  "rollback": { "undoLimit": 10, "bookPointLimit": 3 },
  "grammar": { "endpoint": "" },
  "concurrency": { "mode": "file", "daemon": { "socket": "" } },
  "asciiui": { "types": { "panel": "" } }
}
```

Defaults: `_CLABOK`, `.temp`, `Docs`, 10, 3, `""`, `file`, `""`, no type overrides.
`grammar.endpoint == ""` means "local daemon default", resolved by S13. `daemon.socket == ""` means
"OS default path", resolved by S06. `asciiui` values are empty by default; S09 owns the generic
catalog.

Validation: `version == 1`; roots non-empty after trim and not `.`; limits `>= 0`; mode in
`file|daemon`; endpoint empty or an `http`/`https` URL; asciiui names non-empty.

## 3. Packages and files

```
internal/jsonstrict/decode.go   BOM strip, single document, duplicate-key detection, optional strict fields
internal/config/config.go       File schema (pointer leaves), decode, encode, defaults, prune
internal/config/load.go         discovery, read, resolve, Values
internal/config/keys.go         key registry: get, set, unset, default
internal/config/validate.go     aggregate Issue list, issuesPayload
internal/config/command.go      config show|get|set|unset|validate|path, payloads, atomic write
```

`jsonstrict` is a real responsibility, not a utility grab bag: both the config and the book manifest
depend on it.

## 4. Tasks

1. `jsonstrict.Decode` + tests (duplicates nested, trailing data, BOM, unknown fields).
2. Config file schema, defaults, encode/decode, atomic write.
3. Validation with aggregated issues and stable codes.
4. Discovery and resolution; precedence flags > file > defaults.
5. Key registry and `get`/`set`/`unset` with typed parsing.
6. `config` command group with JSON and text output.
7. Tests per acceptance criterion, including a golden `show`.
8. `gofmt`, `go vet`, `go build`, `go test`, `go test -race`.

## 5. Error codes

`config.not-found`, `config.read-failed`, `config.write-failed`, `config.invalid-json`,
`config.duplicate-key`, `config.unknown-key`, `config.version-missing`,
`config.version-unsupported`, `config.invalid-value`, `config.invalid-mode`,
`config.invalid-path`, `config.invalid-endpoint`, `config.project-not-found`,
`usage.conflicting-flags`.

## 6. Acceptance mapping

| Scope criterion | Evidence |
|---|---|
| 1 unknown key rejected | keys test |
| 2 version required/unsupported | decode + validate tests |
| 3 discovery matrix | discover tests (cwd, --project, --config, not found) |
| 4 relative roots resolve against the config | resolution test from a subdirectory |
| 5 three-layer precedence | precedence test |
| 6 set validates before write | failure leaves the old bytes |
| 7 show = effective values, path = resolved file | golden tests |
| 8 dry run leaves the file byte-identical | hash comparison |
| 9 only `internal/config` opens the file | grep/layering review |
| 10 config errors precede book writes | later scopes call `config.Load` first; contract test |

## 7. Deferred

- Config file version migration: only version 1 exists; the error path is implemented.
- Any key beyond the ones `PLAN.md` §3.11 lists: added by the owning scope's schema version.

## 8. Implementation notes (2026-11-05)

- A missing file is not an error: `show`, `get`, `validate` and `path` use the built-ins;
  `config set` creates the file with `version: 1`; an explicit `--config` that does not exist is
  `config.not-found` for reads.
- `get` and `show` return resolved absolute paths; `set` and `unset` receipts report the stored
  values, so a relative path stays relative in the receipt.
- `unset` on a default is a success with `changed: false` and no write.
- `config path` is tolerant and reports `exists: false` instead of failing.
- `asciiui.types.<name>` with an empty value removes the override; `get` then reports the generic
  name with source `default`.
- `set` on a project without a file writes even when the value equals the default, so the project
  becomes configured.
- Unknown keys passed to `get`/`set`/`unset` are `usage.unknown-key` (exit 2); unknown fields inside
  the file are `config.unknown-key` (failure).

