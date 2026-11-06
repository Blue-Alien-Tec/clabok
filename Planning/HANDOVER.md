# Handover: State After S11 (M5 implemented and pushed; bump/CI deferred)

Status: handover (2026-11-05). Read this first in a fresh session.
Normative source: [../PLAN.md](../PLAN.md). Scope map: [README.md](README.md).

## 1. Where the project stands

- Scopes S01–S12d are implemented. Milestones M1–M4 are delivered. M5 (S11) is implemented and
  pushed on 2026-11-05 **without a version bump and without a CI run**, per the owner decision; the
  `1.1.0-dev` bump and the resulting CI run are deferred. M6 (S12a–S12d) is implemented and the
  `1.2.0-dev` bump is applied; the milestone commit/push is pending.
- The version is `1.2.0-dev` in `internal/buildinfo/buildinfo.go` (M6 close). Changing it is the only
  CI trigger; do it only when the CI run is wanted.
- The previous `HEAD` was `529151d` ("Record the M4 handover and pre-plan the remaining scopes");
  the S11 work is the current commit on `origin/main`. The M4 commit is `5e9fe95`.
- The working tree carries the S12 editorial work (uncommitted): `internal/editorial`, the prose,
  region, asciiui, status and badge commands, manifest v3 with `Node.ProseBlock`, the status
  vocabulary and assignment fields, declared repository badges, the table book index, and the ASCII
  UI layout authoring with its open/close packing. The index-prose schema was corrected on
  2026-11-06 (the text is a block, not a node field). `_CLABOK/` is the working store and is
  gitignored.
- Next: the S12d slice is done and the `1.2.0-dev` bump is applied. Commit/push the M6 milestone when
  the owner asks; the deferred M5 `1.1.0-dev` bump stays deferred. S12e (repository-layout block) is
  the first task of the next planning segment.

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
internal/editorial        prose, region, asciiui, status and badge commands (footer, notes, layout)
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
  navigation. The book index shape is derived from the data: a list without index prose and status,
  a `Document | Description` table once prose (or status) exists; a `part` starts a new table group,
  hierarchy is a `&emsp;` indent, and the per-file `**Contents**` ToC stays a list without prose.
  Index, ToC, reference and Document Index link text is plain text: inline code, emphasis and links
  in a title are stripped so no link contains a second link; headings keep their inline formatting.
- **Refs** (`internal/refs`): `Anchor`, `Assigner`, `Resolve`, `Pattern`. `Resolve` is book-local;
  cross-book resolution is a render concern.
- **Library** (`internal/library`): `FromEnv`, `NewBook`, `Move`, `Rename`, `Unregister`, `Defaults`,
  `SetDefault`, `UnsetDefaults`, `Index` (the `render.CatalogFunc`). Defaults apply at book creation
  only; a book renders deterministically from its own manifest.
- **ID space** (`internal/idspace`): `New(life, layout)`, `Next`, `Peek`. `Next` reads and advances
  the persisted ledger `<store>/ids.json` under the `KindIDs` lock; `Peek` previews without writing
  for dry runs. The ledger is initialized from the highest owned ID of every registered book when
  it is missing.
- **Editorial** (`internal/editorial`): `prose set|clear`, `region set|clear|notes|remove`,
  `asciiui add|set|remove` and `status enable|disable|set|clear|show`. The mutations live in
  `structure`
  (`SetProse`, `SetFooter`, `ClearFooter`, `AddNote`, `RemoveNote`, `AddLayoutBlock`, `SetLayout`,
  `RemoveLayout`, `SetStatus`, `SetStatusEnabled`); created and updated region
  bodies are written in the same atomic step. Manifest v3 adds `Node.ProseBlock`, which references a
  block owned by a `prose` region targeting the node; v2 manifests are read and upgraded in memory.
  The book index shape is derived: a list without prose and status, a `Document | Status |
  Description` table once status is on, a `Document | Description` table once prose exists; the
  per-file ToC stays a list and never carries prose or a marker.
- **Status** (`internal/model/status.go`, `internal/editorial/status.go`): the tool owns the fixed
  vocabulary (`final`, `review`, `wip`, `draft`, ordered high to low, each with a label, a badge
  color and a one-line description). `Manifest.Status.Enabled` is the per-book switch; `Node.Status`
  and `BlockMeta.Status` hold a state id. Rendering emits a shields.io badge in the heading, the
  index `Status` cell and before a block body, plus a `**Status**` legend with badge and meaning
  above the index in every mode. An index column exists only when it has a source: `Document |
  Status` without prose, `Document | Status | Description` with prose. A disabled book renders
  nothing and keeps its assignments, so switching it off is lossless. An unknown assignment is
  `model.unknown-status` and blocks `check`.
- **Badges** (`internal/badge`, `internal/editorial/badge.go`): `Manifest.Badges` declares `id`,
  `label`, optional `message` and optional `link`. The fixed detectors are `go`, `module`, `license`,
  `remote` and `version`; an explicit message wins, and a missing source omits the badge. The
  `badge add|remove|show` command manages them. The badge line renders only in the library README,
  above the H1, as shields.io images wrapped in their link; the resolved values are part of the
  input hash, so a `go.mod`, `LICENSE` or `.git/config` change dirties the README. `check` reports
  `badge-unresolved` as a non-blocking finding.
- **Asciiui** (`internal/asciiui`): `Parse`, `Validate`, `Render`, `Defaults`, `Resolve`; specs live
  at `layouts/asciiui/<blockID>.asciiui` in the container/extract. The drawing is rendered inside a
  fenced `text` code block between the `asciiui` markers, so Markdown keeps its spaces and line
  breaks. The built-in defaults are generic names only; `asciiui.types.<kind>` names the project
  type and always wins over an explicit generic label, while an explicit non-generic label stays a
  one-node override. `widget=` names a UI widget type, never a content or domain object. A framed
  panel stretches to a row's height and to a stack's width, and a separator fills its panel. The
  authoring commands
  `asciiui add|set|remove` are registered from `editorial` and run through `structure`
  (`AddLayoutBlock`, `SetLayout`, `RemoveLayout`) in one atomic step. Open extracts the specs and
  close packs them, so a closed book renders the same drawings.
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
  `structure`, `editorial`, `library`, `render`) carry command surfaces and do.
- Determinism: no clock, map iteration or absolute paths inside the container; JSON maps sorted.

## 7. Known gaps and explicit carry-overs

- **Milestone bump/CI deferred:** S11 is pushed without a buildinfo change, so no CI run was
  triggered. Bump to `1.1.0-dev` in a follow-up commit when the CI run is wanted.
- **Apply has no content step.** S07b planned it for S08; S08 defined the body commands but no
  `apply` body op exists. Decide in a later scope; the gate (S13) must then cover it.
- **Editorial state is partial.** S12a delivers manifest v3, index prose, the footer and notes
  commands and their rendering; S12b delivers the ASCII UI authoring commands, the locked layout
  anchor and the open/close packing of the specs; S12c delivers the status system, assignments, the
  index status column and the legend; S12d delivers the badge detectors, the badge commands and the
  library badge line. S12e remains: the repository-layout block, deferred to the next planning
  segment.
- **Index prose is a block reference.** S12a originally stored the prose as the node text field
  `Node.Prose`, which contradicts `PLAN.md` §3 statement 5. Since 2026-11-06 the text lives in a
  block referenced by `Node.ProseBlock` and owned by a `prose` region; the node carries no prose
  text. This is a schema correction, not a new feature.
- **No grammar/terms code** exists; the body write path is the single hook point.
- **No TUI** exists.
- **Legacy books** under `Assets/Docs` are untouched and unreadable by CLABOK (no import/adopt).
- The daemon never auto-starts; `pecia serve` is explicit. The grammar server (S13) is a separate
  process.
- The cut-over store (`_CLABOK/`) is not tracked. A fresh clone has no library book until the
  runbook in `plans/11-library-docs-and-cutover.md` §6 is replayed.

## 8. Starting the next session

1. Read this file, `README.md` §4/§4.1 and the S12 scope document.
2. `git status` shows the uncommitted S12 work; `HEAD` is the S11 commit; `Version` is `1.2.0-dev`.
3. Run `go test ./... -count=1` and `go build -o pecia.exe ./cmd/pecia`, then
   `.\pecia.exe sync --check` (must be clean).
4. Commit and push the M6 milestone when the owner asks. S12e (repository-layout block) is the first
   task of the next planning segment.
