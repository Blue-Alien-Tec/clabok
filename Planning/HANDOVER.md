# Handover: State After M4

Status: handover (2026-11-05). Read this first in a fresh session.
Normative source: [../PLAN.md](../PLAN.md). Scope map: [README.md](README.md).

## 1. Where the project stands

- Scopes S01–S10 are implemented. Milestones M1–M4 are delivered.
- The version is `1.0.0-dev` in `internal/buildinfo/buildinfo.go`.
- `HEAD` is `5e9fe95` ("Finish M4: …"), pushed to `origin/main`; CI run `42106203034` is green.
- Next: S11 (M5, version `1.1.0-dev`), then S12/S13 (M6/M7, `1.2.0-dev`/`1.3.0-dev`),
  S14 (M8, `1.4.0-dev`).

## 2. Non-negotiable workflow rules

- **One commit per milestone.** The milestone work and the version bump live in the same commit.
  Slices stay local. Do not commit and do not push without a bump.
- **One push, one CI run.** The workflow triggers only on a `main` push that changes
  `internal/buildinfo/buildinfo.go` (plus `workflow_dispatch`). Any other push must not happen.
- The local verifies before the bump: `gofmt -l .` empty, `go vet ./...`, `go build ./...`,
  `go test ./... -count=1`.
- The earlier unbumped commit `c17c01e` remains in history by decision; do not revert or
  force-push it (that would trigger another run).
- Never hand-edit `.unity`, `.prefab`, `.asset` or `.meta`; not relevant for CLABOK.

## 3. Toolchain

- Go 1.27.3 at `C:\Users\btook\AppData\Local\Programs\Go\bin\go.exe`. It is not on the agent shell's
  `PATH`; prepend that directory in each shell call.
- `go test -race` needs cgo on Windows (no C toolchain here); the Linux CI job covers race.
- `scripts/check.ps1` is blocked by the local execution policy; run its four commands directly.
- Build the binary with `go build -o pecia.exe ./cmd/pecia` (gitignored).

## 4. Repository map

```
cmd/pecia                 wiring: registry + Register calls + cli.Run
internal/buildinfo        Product, CLI, Version (ldflags target)
internal/cli              envelope, exit codes 0/1/2/3, reject, hints, dry-run, Texter
internal/jsonstrict       strict JSON (BOM, duplicate keys, trailing data, unknown fields)
internal/config           clabok.json, discovery, precedence, six config commands
internal/model            Manifest v2, nodes, regions, BlockMeta, IDs, validators
internal/store            deterministic container, layout, registry, hashes, atomic writes
internal/rollback         undo/book/body packages, caps, names
internal/coordinator      shared/exclusive locks (file + AF_UNIX daemon), serve
internal/lifecycle        open/close, extraction, snapshots, restore, locks, FromEnv
internal/structure        add/rename/move/remove/type/rekey/apply-runner
internal/apply            declarative plan v1
internal/body             body get/set/append/prepend/replace/list, block-0 creation
internal/prose            shared prose rule (struct headings forbidden)
internal/search           find over titles and bodies
internal/temp             .temp listing and flush
internal/asciiui          catalog, spec parser, ASCII layout engine
internal/refs             title anchors, assigner, target resolution, [[id:N]]
internal/render           pure renderer, sync, check/verify
```

## 5. Contracts a new scope must use

- **Model** (`internal/model`): `Manifest` version 2, `BlockMeta{Type, Locked}` in `BlockMeta`
  map, `Validate()` aggregates `model.Issue`s. Placement lists are the existence of blocks.
- **Store** (`internal/store`): `Container` (STORE, fixed time, checksums), `Layout`
  (`<root>/<id>/<id>.book`, `blocks/`, `layouts/`, `state.json`, `library.json`, `terms.txt`,
  `open.json`, `rollback/<id>/`, `.coordinator/`), `Registry{Version, Books[{ID, Path, Kind}]}`,
  `State{InputHash, OutputHash}`, `WriteAtomic`, `OpenBookCount`.
  The registry `Path` is the **output directory** relative to the project; the library book uses
  `.` so the monolith writes `<project>/README.md`.
- **Lifecycle** (`internal/lifecycle`): `FromEnv`, `RequireOpen`, `RequireClosed`, `State`,
  `Snapshot` (whole state), `SnapshotBody` (one block), `Restore` (undo/body/book), `CollectMembers`,
  `LockBookExclusive`, `LockBookShared`, `LockBlock`.
- **Coordinator** (`internal/coordinator`): `Acquire(resources, mode, timeout)`; lock order
  book → node → block; structure commands hold the book exclusively, body writes hold the book
  shared plus one block, open/close hold store+book exclusively.
- **Structure** (`internal/structure`): `Add`, `AddTyped`, `AddBlock`, `AddLockedBlock`,
  `Rename`, `Move`, `Remove`, `SetBlockType`, `Rekey`, `Execute(steps)`. Every mutation snapshots,
  validates, and reports `book.invalid` as a reject.
- **Body** (`internal/body`): `Get`, `List`, `Set`, `Append`, `Prepend`, `Replace`. Block `0`
  creates a block; locked blocks reject body and type changes; every write uses a body-scoped undo.
- **Render** (`internal/render`): `Render` is pure; `Sync(book, outputDir, SyncOptions)` supports
  `--check/--write/--prune/--force`; `Check` returns findings. `Marker` is the first line of every
  generated file. `Slug`/anchors derive from titles; references use `[[id:N]]`.
- **Refs** (`internal/refs`): `Anchor`, `Assigner`, `Resolve`, `Pattern`.
- **Asciiui** (`internal/asciiui`): `Parse`, `Render(spec, overrides)`, `Defaults`, `Resolve`.
  The renderer reads `<book>/layouts/asciiui/<blockID>.asciiui`.
- **Apply** (`internal/apply`): plan v1 with ops `add`, `rename`, `move`, `remove`, `type`,
  `setting`. It has **no body step** (see gaps).
- **Config** (`internal/config`): `Values` with ProjectDir, StoreRoot, TempRoot, DocsRoot,
  UndoLimit, BookPointLimit, GrammarEndpoint, ConcurrencyMode, DaemonSocket, AsciiUITypes.
- **CLI** (`internal/cli`): `Command{Name, Flags, Run}`, `Registry`, `Run`; `Result{Data, Text}`;
  payloads implement `Text()` and mutating receipts implement `DryRunResult()`.

## 6. Invariants and conventions

- IDs: `uint64`, encoded as decimal **strings** in JSON; zero is the block-creation sentinel and is
  never stored; IDs are globally unique and library-managed. `rekey` is the only ID change.
- Structure: exactly three levels (title/chapter/subchapter); blocks are placed IDs with no
  children; metadata is an overlay; types may repeat; `locked` protects the body and the type only,
  while moves and removals stay allowed and a removal drops the metadata and any notes whose target
  disappears.
- Rendering: README always exists; headings are generated; an explicit `<a id="…">` makes anchors
  number-independent; prose `####` is text; one blank line between blocks; dangling `[[id:N]]` stays
  literal and is a blocking finding.
- Undo: every mutation snapshots; body writes snapshot one block; close clears undo and body
  packages; close creates a book point only when the repacked bytes differ.
- Tests: table-driven, `t.TempDir` scenes, no network, no flaky sleeps except lock timeouts; command
  tests go through `cli.Run`; scene helpers build the manifest/container and open the book.
- Layering: pure packages (`buildinfo`, `jsonstrict`, `model`, `store`, `rollback`) never import the
  CLI; `config`, `lifecycle`, `coordinator`, `structure`, `apply`, `body`, `search`, `temp`,
  `render` carry command surfaces and do.
- Determinism: no clock, map iteration or absolute paths inside the container; JSON maps are sorted
  by `encoding/json`.

## 7. Known gaps and explicit carry-overs

- **Apply has no content step.** S07b planned it for S08; S08 defined the body commands but no
  `apply` body op was added. Decide in M5 or a follow-up; the gate (S13) must then cover it.
- **Rekey is book-local.** Library-wide rewriting waits for S11; it should iterate the registry and
  require the affected books to be open (reject with the list of closed books instead of opening
  them as a side effect).
- **Index prose is not modelled.** The renderer's index has no prose field; S12 adds it.
- **Index prose and the asciiui authoring commands** are the two S12 features the renderer already
  has contracts for.
- **No grammar/terms code** exists; the body write path has the single hook point.
- **No library code** exists; `book new`, `library sync`, move/rename/unregister and the README
  cut-over are all S11.
- **No TUI** exists.
- The daemon never auto-starts; `pecia serve` is explicit. The grammar server (S13) is a separate
  process.

## 8. Starting the next session

1. Read this file, `README.md` §4/§4.1 and the S11 scope document.
2. `git status` must be clean; `HEAD` is `5e9fe95`; `Version` is `1.0.0-dev`.
3. Run `go test ./... -count=1` before changing anything.
4. Write the S11 implementation plan under `Planning/plans/11-…`.
5. Implement S11. Keep every change local.
6. At the end: bump to `1.1.0-dev`, one commit with all work, one push, watch the single CI run.
