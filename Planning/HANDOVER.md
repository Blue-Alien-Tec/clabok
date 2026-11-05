# Handover: State After S11 (M5 implemented and pushed; bump/CI deferred)

Status: handover (2026-11-05). Read this first in a fresh session.
Normative source: [../PLAN.md](../PLAN.md). Scope map: [README.md](README.md).

## 1. Where the project stands

- Scopes S01–S11 are implemented. Milestones M1–M4 are delivered. M5 (S11) is implemented and
  pushed on 2026-11-05 **without a version bump and without a CI run**, per the owner decision; the
  `1.1.0-dev` bump and the resulting CI run are deferred.
- The version is still `1.0.0-dev` in `internal/buildinfo/buildinfo.go`. Changing it is the only
  CI trigger; do it only when the CI run is wanted.
- The previous `HEAD` was `529151d` ("Record the M4 handover and pre-plan the remaining scopes");
  the S11 work is the current commit on `origin/main`. The M4 commit is `5e9fe95`.
- The working tree is clean after the push. `_CLABOK/` is the working store and is gitignored.
- Next: bump to `1.1.0-dev` when the CI run is wanted, then S12/S13 (M6/M7,
  `1.2.0-dev`/`1.3.0-dev`), then S14 (M8, `1.4.0-dev`).

## 2. Non-negotiable workflow rules

- **One commit per milestone.** The version bump normally rides in the milestone commit. S11 was
  pushed without it by the owner's explicit decision, so no CI ran; bump in a follow-up commit when
  the CI run is wanted.
- **CI trigger.** The workflow triggers only on a `main` push that changes
  `internal/buildinfo/buildinfo.go` (plus `workflow_dispatch`). A push without a buildinfo change
  does not start a run.
- The local verifies before the bump: `gofmt -l .` empty, `go vet ./...`, `go build ./...`,
  `go test ./... -count=1`. The cut-over acceptance is `pecia sync --check` (exit 0, clean).
- The earlier unbumped commit `c17c01e` remains in history by decision; do not revert or
  force-push it (that would trigger another run).
- Never hand-edit `.unity`, `.prefab`, `.asset` or `.meta`; not relevant for CLABOK.
- Never hand-edit the generated `README.md`; edit the library book through the CLI and run
  `pecia sync --write`.

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
internal/store            deterministic container, layout, registry v2, hashes, atomic writes
internal/rollback         undo/book/body packages, caps, names
internal/coordinator      shared/exclusive locks (file + AF_UNIX daemon), serve
internal/lifecycle        open/close, extraction, snapshots, restore, locks, FromEnv
internal/structure        add/rename/move/remove/type/apply-runner
internal/apply            declarative plan v1
internal/body             body get/set/append/prepend/replace/list, block-0 creation
internal/prose            shared prose rule (struct headings forbidden)
internal/search           find over titles and bodies
internal/temp             .temp listing and flush
internal/asciiui          catalog, spec parser, ASCII layout engine
internal/refs             title anchors, assigner, target resolution, [[id:N]]
internal/render           pure renderer (open or closed source), sync, check, library-wide sync
internal/library          registry defaults, book new/move/rename/unregister, Document Index
internal/idspace          library ID ledger, global allocation, Peek
```

## 5. Contracts a new scope must use

- **Model** (`internal/model`): `Manifest` version 2, `BlockMeta{Type, Locked}`, `Validate()`
  aggregates `model.Issue`s. `Settings.Normalized()` fills index depth and output.
- **Store** (`internal/store`): `Container` (STORE, fixed time, checksums), `Layout`
  (`<root>/<id>/<id>.book`, `blocks/`, `layouts/`, `state.json`, `library.json`, `ids.json`,
  `terms.txt`, `open.json`, `rollback/<id>/`, `.coordinator/`), `Registry` **version 2**
  (`{version, books:[{id, path, kind}], defaults?}`), `State{InputHash, OutputHash}`, `WriteAtomic`.
  The registry `Path` is the output **directory** relative to the project; the library entry uses
  `.`. Paths are relative, clean and never escape the project. The registry stores no title, index
  file name or note.
- **Lifecycle** (`internal/lifecycle`): `FromEnv`, `Paths`, `RequireOpen`, `RequireClosed`, `State`,
  `Snapshot`, `SnapshotBody`, `Restore`, `CollectMembers`, `LockStore`, `LockIDs`,
  `LockBookExclusive`, `LockBookShared`, `LockBlock`. Lock order: store → book → node → block.
- **Coordinator** (`internal/coordinator`): `Acquire(resources, mode, timeout)`; the coordinator
  sorts resources, so multi-book acquisitions cannot deadlock.
- **Structure** (`internal/structure`): `Add`, `AddTyped`, `AddLink`, `AddBlock`, `AddLockedBlock`,
  `Rename`, `Move`, `Remove`, `SetBlockType`, `Execute(steps)`,
  `ApplySettingValue(settings, key, value)`. Every mutation snapshots, validates and reports
  `book.invalid` as a reject.
- **Body** (`internal/body`): `Get`, `List`, `Set`, `Append`, `Prepend`, `Replace`. Block `0`
  creates a block; locked blocks reject body and type changes.
- **Render** (`internal/render`): `Render` is pure; the source is the open extract when extracted and
  the container otherwise. This is the **sole** read path that bypasses the open state (`PLAN.md`
  §3.5); body and structure mutation still require `open`. `Sync(book, outputDir, SyncOptions)`
  supports `--check/--write/--prune/--force`. `Sync` without a book renders the whole registry
  (`RunLibrarySync`, also used by `library sync`). `Service.LibraryIndex` injects the Document
  Index; it participates in the input hash. `Marker` is the first line; marker matching is
  book-scoped. `Check` is read-only and uses the same source. `RunLibrarySync` first rejects a
  duplicate owned ID across books (`library.id-duplicate`) before rendering anything.
  `LayoutFor(manifest)` is the single anchor/file source. `Service.CrossLink`/`CrossBook` resolve
  `[[id:N]]` and link entries in another registered book; `buildCrossIndex` reads the registered
  manifests (no bodies, no open) and each book gets hrefs relative to its output directory.
  Split modes with `toc` enabled emit a per-file `**Contents**` before the H1; link entries render
  as links. Navigation is visually DocEngine-identical: `← Previous`, `Index`, `Next →` at the top
  and bottom of split files with `---` rules (absent directions omitted); a book README carries
  `[← Library]` plus `[First document →]` in split modes; the library README carries
  `[Document Index](#document-index)` at the top and bottom. The footer sits above the bottom
  navigation.
- **Refs** (`internal/refs`): `Anchor`, `Assigner`, `Resolve`, `Pattern`. `Resolve` is book-local;
  cross-book resolution is a render concern.
- **Library** (`internal/library`): `FromEnv`, `NewBook`, `Move`, `Rename`, `Unregister`, `Defaults`,
  `SetDefault`, `UnsetDefaults`, `Index` (the `render.CatalogFunc`). Defaults apply at book creation
  only; a book renders deterministically from its own manifest.
- **ID space** (`internal/idspace`): `New(life, layout)`, `Next`, `Peek`. `Next` reads and advances
  the persisted ledger `<store>/ids.json` under the `KindIDs` lock; `Peek` previews without writing
  for dry runs. The ledger is initialized from the highest owned ID of every registered book when
  it is missing.
- **Asciiui** (`internal/asciiui`): `Parse`, `Render`, `Defaults`, `Resolve`; specs live at
  `layouts/asciiui/<blockID>.asciiui` in the container/extract.
- **Apply** (`internal/apply`): plan v1 with `add`, `rename`, `move`, `remove`, `type`, `setting`;
  still **no body step**.
- **Config** (`internal/config`): `Values` with ProjectDir, StoreRoot, TempRoot, DocsRoot,
  UndoLimit, BookPointLimit, GrammarEndpoint, ConcurrencyMode, DaemonSocket, AsciiUITypes.
- **CLI** (`internal/cli`): `Command{Name, Flags, Run}`, `Registry`, `Run`; **command names are at
  most two words**. A group with more actions takes a positional action (for example
  `library defaults show|set|unset`).

## 6. Invariants and conventions

- IDs: `uint64`, decimal strings in JSON; zero is the block-creation sentinel. The library owns the
  global namespace: every issued ID comes from the persisted ledger (`<store>/ids.json`) via
  `idspace.Allocator.Next`. Uniqueness is a constraint, not a probability. IDs are immutable:
  `move`/reparent change only the ordered collections, a copy gets a new ID, and there is no rekey.
  A duplicate owned ID across books is corruption: library-wide `sync` rejects it as
  `library.id-duplicate` with the ID and the books; it is never repaired by renaming.
- Structure: exactly three levels (title/chapter/subchapter); blocks are placed IDs; `locked`
  protects body and type; moves/removals stay allowed.
- Registry: one library entry with path `.`; unique IDs and paths; the library entry is written
  first; entry order (minus the library) is the Document Index order. A registry change does not
  rewrite books; it makes the library README dirty through the index hash.
- Rendering: README always exists; headings are generated; anchors are number-independent; dangling
  `[[id:N]]` is a blocking finding; `####` in prose is text.
- Drift: `sync --check` fails with `render.drift` when the explicit flag is used and any book
  drifts; an implicit dry run stays informational. `--check` never creates the output directory.
  Foreign/prune detection matches the generated marker's book line, never a whole-directory scan.
- Open/closed equivalence: an open and a closed book render identical files and hash identically.
- Body and structure mutation require an open book; render, `check` and `verify` are the sole closed
  read path. `book move`, `book rename` and `book unregister` require a closed book and touch only
  the registry or the manifest title.
- Undo: every mutation snapshots; body writes snapshot one block; close clears undo/body packages.
- Tests: table-driven, `t.TempDir` scenes, no network, command tests through `cli.Run`.
- Layering: pure packages (`buildinfo`, `jsonstrict`, `model`, `store`, `rollback`, `refs`,
  `asciiui`) never import the CLI; command packages (`config`, `lifecycle`, `coordinator`,
  `structure`, `library`, `render`) carry command surfaces and do.
- Determinism: no clock, map iteration or absolute paths inside the container; JSON maps sorted.

## 7. Known gaps and explicit carry-overs

- **Milestone bump/CI deferred:** S11 is pushed without a buildinfo change, so no CI run was
  triggered. Bump to `1.1.0-dev` in a follow-up commit when the CI run is wanted.
- **Apply has no content step.** S07b planned it for S08; S08 defined the body commands but no
  `apply` body op exists. Decide in a later scope; the gate (S13) must then cover it.
- **Index prose is not modelled.** The renderer's Document Index has one column; S12 adds prose and
  the note column. The repository-layout block is S12.
- **No grammar/terms code** exists; the body write path is the single hook point.
- **No TUI** exists.
- **Legacy books** under `Assets/Docs` are untouched and unreadable by CLABOK (no import/adopt).
- The daemon never auto-starts; `pecia serve` is explicit. The grammar server (S13) is a separate
  process.
- The cut-over store (`_CLABOK/`) is not tracked. A fresh clone has no library book until the
  runbook in `plans/11-library-docs-and-cutover.md` §6 is replayed.

## 8. Starting the next session

1. Read this file, `README.md` §4/§4.1 and the S12 scope document.
2. `git status` is clean; `HEAD` is the S11 commit; `Version` is still `1.0.0-dev`.
3. Run `go test ./... -count=1` and `go build -o pecia.exe ./cmd/pecia`, then
   `.\pecia.exe sync --check` (must be clean).
4. Bump `internal/buildinfo/buildinfo.go` to `1.1.0-dev` in a follow-up commit when the CI run is
   wanted, then plan and implement S12.
