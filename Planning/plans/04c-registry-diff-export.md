# S04c — Implementation Plan: Registry, Diff and Export

Status: implemented (2026-11-05). Parent scope: [../04-central-store-and-containers.md](../04-central-store-and-containers.md).
Normative source: [../../PLAN.md](../../PLAN.md) §3.2, §3.7 (store layout), §5.

Third and last delivery slice of S04. Registry *policy* stays with S11; this slice owns the file
format primitives only.

## 1. Decisions

| # | Question | Decision | Rationale |
|---|---|---|---|
| 1 | Registry shape | `{"version": 1, "books": [{"id", "path", "kind"}]}` | Minimal data the store needs; S11 adds policy fields with a version bump |
| 2 | Kinds | `library` and `book` | `PLAN.md` §2.1 uses exactly these |
| 3 | Path semantics | relative to the project directory, non-empty, not absolute | The output location survives a project move; S11 owns the resolution rules |
| 4 | Decode strictness | tolerant unknown fields, version checked after decode | A newer registry is rejected by its version, not by a field name |
| 5 | Validation | version, valid and unique IDs, non-empty relative path, known kind | First problem is reported with a stable code; the registry is small |
| 6 | Persistence | atomic write, deterministic encoding, entry order preserved | Entry order is the library index order (S11); sorting is not the store's call |
| 7 | Missing file | an empty registry at the current version | A fresh project is usable without a ceremony command |
| 8 | `Diff` | member-level `added`, `changed`, `removed`, sorted by name | What `sync --check` and review output need |
| 9 | `Export` | writes every member into a target directory with the member tree, atomic per file | Inspection and recovery without opening a book |
| 10 | Export safety | a computed target outside the directory is rejected | Defence in depth on top of member-name validation |

## 2. Error codes

`store.registry-corrupt`, `store.registry-version`, `store.registry-invalid`,
`store.invalid-root`, `store.write-failed`, `store.read-failed`, `store.member-name`.

## 3. Tasks

1. `Registry`, `RegistryEntry`, kinds, `LoadRegistry`, `Save`, `Validate`.
2. `Find`, `Upsert`, `Remove`.
3. `Diff(before, after)` with sorted changes.
4. `Container.Export(directory)`.
5. Tests for round-trip, strictness, validation, diff and export.
6. `gofmt`, `go vet`, `go build`, `go test`.

## 4. Acceptance

| S04 criterion | Evidence |
|---|---|
| 7 `diff` reports added, changed and removed members | diff test |
| 7 `export` writes exactly the member tree | export test, nested member |
| 8 registry persistence with a version field | round-trip and version-rejection test |
| 10 atomic writes | no temp leftovers after save/export |

## 5. Out of scope

- Library policy, defaults and the `library` commands: S11.
- Which commands call `diff`/`export`: S09/S11.
- Open/close and locking: S05, S06.

## 6. Implementation notes (2026-11-05)

- The registry decoder tolerates unknown fields and rejects an unsupported version afterwards, so a
  newer registry fails on its version rather than on a field name.
- Output paths are stored relative to the project directory and an absolute path is rejected. S11
  owns resolution and the docs-root rule.
- Entry order is preserved on upsert and removal; it is the library index order.
- `Diff` reads the internal member maps directly, so it does not copy bodies.
- `Export` writes each member through the shared atomic writer and rejects a computed target outside
  the export directory as defence in depth.
- `Registry.Save` forces the current version before validation, so a caller cannot persist a future
  version by accident.

