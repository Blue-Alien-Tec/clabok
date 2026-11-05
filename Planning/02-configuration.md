# S02 — Configuration

Status: implemented (2026-11-05). Plan: [plans/02-configuration.md](plans/02-configuration.md).
Phase: 1. Depends on: S01. Blocks: S04 (paths), S06 (concurrency mode), S09 (asciiui overrides),
S11 (docs root), S13 (grammar endpoint).
PLAN refs: §3.11, §3.6 (docs root), §6 (validated before any write, paths relative to the config
file).

## 1. Goal

Provide one strict, versioned project configuration that resolves the store root, temp root, docs
root, limits, grammar endpoint, concurrency backend and asciiui type overrides, with a complete
command surface and a precedence rule that every other scope can rely on.

## 2. In scope

| # | Requirement | PLAN |
|---|---|---|
| 1 | One project config file `clabok.json` at the project root, JSON via the standard library | §3.11 |
| 2 | Configurable paths: store root (default `_CLABOK`), temp root (default `.temp`), docs root (default `Docs`). The library `README.md` stays fixed at the project root and is not configurable | §3.11, §3.6 |
| 3 | Configurable limits: undo rollback packages (default 10) and book rollback points (default 3) | §3.11 |
| 4 | Configurable grammar endpoint; default is the local LanguageTool daemon in the OS temp directory | §3.11 |
| 5 | Configurable coordination: `concurrency.mode` = `file` (default) or `daemon`, plus an optional daemon socket override | §3.11, §3.12 |
| 6 | `asciiui.types`: generic name -> optional project-specific override; blank or absent keeps the generic default; defaults contain no project-specific names | §3.11 |
| 7 | Precedence: CLI flags > config file > built-in defaults | §3.11 |
| 8 | Relative paths resolve against the directory of the config file, not the working directory | §3.11 |
| 9 | Discovery: `--project <dir>` or `--config <file>`; otherwise search upward from the working directory; no implicit global config | §3.11 |
| 10 | Strict loading: unknown keys rejected, config version required, an invalid config fails before any write | §3.11 |
| 11 | Commands: `config show`, `get <key>`, `set <key> <value>`, `unset <key>`, `validate`, `path`; JSON by default, `--text`; `set`/`unset` accept `--dry-run`; project-level, no open book required | §3.11 |
| 12 | One resolved-value type published to the other scopes; no scope re-reads the file on its own | §3.11 |

## 3. Out of scope

- Consuming the values: store root and layout S04, temp area S08, docs root S11, limits S05, grammar
  endpoint S13, concurrency backends S06, asciiui catalog S09.
- Writing the config from anywhere except the `config` commands.
- Environment-variable configuration: not stated in `PLAN.md`, so not introduced.
- A global or user-level config: explicitly excluded by `PLAN.md`.

## 4. Deliverables

- `internal/config`: schema types, defaults, strict decoder, version check, path resolution,
  precedence merge, validation errors with stable codes.
- `internal/cli` command group `config` with the six commands and `--dry-run` for `set`/`unset`.
- Atomic write for `set`/`unset` that validates the resulting document before replacing the file.
- Tests for the discovery matrix, precedence matrix, strictness, path resolution and every command.
- Golden JSON output for `config show` and `config path`.
- This scope's implementation plan under `Planning/plans/`.

## 5. Acceptance criteria

1. An unknown key is rejected with a stable error code; nothing is written.
2. A missing `version` is rejected; an unsupported version is rejected.
3. `--config`, `--project` and upward discovery each find the intended file; discovery stops at the
  filesystem root and reports "no config" explicitly instead of guessing.
4. Relative store, temp and docs roots resolve against the config file directory; a test runs the
   binary from a subdirectory with a relative root and asserts the absolute result.
5. A CLI flag overrides the file; the file overrides the built-in default; a test covers all three
  layers for at least one key per category.
6. `config set` on an invalid value fails before any write; a successful `config set` leaves a
  document that `config validate` accepts.
7. `config show` emits the effective values, not the raw file, and `config path` the resolved file.
8. `config set` / `unset --dry-run` leave the file byte-identical.
9. No other package opens `clabok.json` directly (checked by review or a test).
10. Config errors surface before the first book write in every mutating command path.

## 6. Design decisions for this scope's planning session

- Key naming: flat dotted keys (`store.root`) versus nested paths in `get`/`set`. Decide the
  canonical form used by `show`, `get`, `set` and `unset`.
- Value typing: how `set` expresses lists and maps (`asciiui.types`), and whether it accepts JSON
  literals, JSON pointers or single scalars only.
- Whether `config show` includes the source layer per key (useful for debugging, larger output).
- Config file version field: integer, string, and the migration/refusal policy for an older version.
- Where `--project` points when it holds no config: error, or a project root without a file that
  falls back to defaults (decide; `PLAN.md` only fixes validated loading).
- Whether the daemon socket override is a path relative to the config file or an OS-level address.
- Default docs root on a case-insensitive filesystem (`Docs` vs `docs`) and the guard against
  colliding with a book folder of the same name.

## 7. Risks and constraints

- Strict loading means a config written for a newer version is a hard error. Keep the error message
  actionable (file, key, accepted version).
- Path resolution against the config file is the invariant that makes `--project` usable from any
  subdirectory; test it early, because every later scope depends on it.
- The config is not a book and never lives in the store.
