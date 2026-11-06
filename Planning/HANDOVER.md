# Handover: State After S11 (M5 implemented and pushed; bump/CI deferred)

Status: handover (2026-11-05). Read this first in a fresh session.
Normative source: [../PLAN.md](../PLAN.md). Scope map: [README.md](README.md).

## 1. Where the project stands

- Scopes S01–S14 are implemented. Milestones M1–M4 are delivered. M5 (S11) is implemented and
  pushed on 2026-11-05 **without a version bump and without a CI run**, per the owner decision; the
  `1.1.0-dev` bump and the resulting CI run are deferred. M6 (S12a–S12d) is committed on `main`
  (`249b3b5`, `1.2.0-dev`). M7 (S12e + S13) is committed on `main` (`db3cf3e`, `1.3.0-dev`). M8
  (S14) is implemented with the `1.4.0-dev` bump; this commit closes the planned ladder.
- The version is `1.4.0-dev` in `internal/buildinfo/buildinfo.go` (M8 close). Changing it is the only
  CI trigger; do it only when the CI run is wanted.
- The previous `HEAD` was `529151d` ("Record the M4 handover and pre-plan the remaining scopes");
  the S11 work is on `origin/main`. The M4 commit is `5e9fe95`; M6 is `249b3b5`; M7 is `db3cf3e`.
- The M7 commit carries the S12e and S13a–S13d work: the generated repository-layout block,
  `internal/repolayout`, the `layout` command and its validation; the grammar client, findings,
  `grammar.policy`, the body-write gate, `grammar check`, `grammar apply`, the term list and the
  LanguageTool server lifecycle (`serve`, `server-info`, `server-stop`). `_CLABOK/` is the working
  store and is gitignored.
- The M8 commit carries S14a–S14c: `internal/tui` with the view model, intents, the key map, the
  ASCII view renderer, the controller over the core services, the draft-preserving `$EDITOR`
  handoff, `pecia tui` with the TTY guard and the rendered key-map chapter.
- Next: the planned scope ladder is complete. New work starts with a new scope and plan.

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
internal/model            Manifest v3, nodes, regions, BlockMeta, status, badges, layout, validators
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
internal/editorial        prose, region, asciiui, status, badge and layout commands
internal/repolayout       tracked-directory tree from git ls-files, depth/exclude, per-path notes
internal/grammar          LanguageTool client, findings, policy gate, check, apply, server lifecycle
internal/tui              view model, intents, ASCII renderer, controller and `pecia tui`
```

## 5. Contracts a new scope must use

- **Model** (`internal/model`): `Manifest` version 3 (`Node.ProseBlock`, `Node.Status`,
  `BlockMeta.Status`, `Status`, `Badges`, `Layout`), `Validate()` aggregates `model.Issue`s.
  `Settings.Normalized()` fills index depth and output.
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
  `[← Library]`, every other registered book as a direct link (library order, the current book is
  omitted) and `[First document →]` in split modes; the library README lists the registered books
  before `[Document Index](#document-index)` at the top and bottom. The footer sits above the bottom
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
- **Layout** (`internal/repolayout`, `internal/editorial/layout.go`): `Manifest.Layout` holds the
  root, the depth (0 means 2), repository-relative excludes and one-line notes. A locked
  `RepoLayout` block is the single anchor; `layout add|set|exclude|include|note|show` authors it.
  Rendering runs `git ls-files -z` in the project directory, keeps the tracked directories only
  (individual files are not listed), prunes excluded branches, honours the depth and emits the
  directory tree inside a fenced `text` block between `repolayout` markers. The tree participates in
  the input hash. No git work tree renders an informative line, never an error.
- **Grammar** (`internal/grammar`): `Client` posts to `grammar.endpoint` (default
  `http://127.0.0.1:8081/v2/check`, `en-US`, 5s timeout), maps the LanguageTool matches to
  `Finding`s (index, block, line, column, offset, rule, message, replacements) and suppresses terms
  case-insensitively. `Gate.Evaluate` applies `grammar.policy` (`block`, `warn`; there is no `off`)
  and, when the endpoint is unreachable, starts the shared server on demand and checks once more;
  only a start that is impossible degrades to a single `grammar check skipped: …` line. The gate runs
  before the snapshot on every prose write path: `body set|append|prepend`, `body replace`,
  `prose set`, `region set`, `region notes` and `layout note`. `block` rejects with
  `body.grammar-blocked` and the findings as details. `grammar check` is read-only, covers a block, a
  file and raw text, and also starts the server on demand. `grammar apply` re-checks the body,
  applies `--all-safe` or the selected 1-based findings right-to-left by offset, writes once through
  `SnapshotBody`/`WriteAtomic`, guards `--expect`, re-checks the result and reports the remaining
  findings; every replacement is server-provided, so the repair path does not block a partial fix.
  Terms live in `<store>/terms.txt` (one per line, case-insensitive, first form kept); requests live
  in `<store>/pending-terms.json` and never touch the list. `terms set|delete` create a pending
  request with a reason and return its id; `terms pending` lists; `terms commit --id --confirm` is
  the only writer; `terms discard` removes a request; `terms list` reads the list. A finding is
  suppressed when its offending text or a replacement contains a term case-insensitively.
  `grammar serve` derives the base URL from the local endpoint, discovers the jar via `--jar`,
  `CLABOK_LANGUAGETOOL_JAR`/`LANGUAGETOOL_JAR` or the `language_tool_python` caches, starts
  `java -cp <jar> org.languagetool.server.HTTPServer --port <port> --allow-origin *` detached and
  waits for `/v2/languages`; the state, lock and log live in `os.TempDir()/clabok-grammar`
  (`CLABOK_GRAMMAR_STATE_DIR` overrides it in tests). `server-info` probes, `server-stop` stops the
  recorded pid. `TestLanguageToolLive` starts the real server through `Serve` and is skipped when
  Java or a jar is missing.
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
- **S12 is complete.** S12a–S12d are committed on `249b3b5` with the `1.2.0-dev` bump; S12e (the
  repository-layout block) is part of the M7 commit.
- **Index prose is a block reference.** S12a originally stored the prose as the node text field
  `Node.Prose`, which contradicts `PLAN.md` §3 statement 5. Since 2026-11-06 the text lives in a
  block referenced by `Node.ProseBlock` and owned by a `prose` region; the node carries no prose
  text. This is a schema correction, not a new feature.
- **S13 is complete.** S13a–S13d deliver the grammar client, the gate on every prose path,
  `grammar check|apply`, the term list with the confirmation gate, and the LanguageTool server
  lifecycle; the M7 commit carries them with the `1.3.0-dev` bump.
- **S14 is complete.** S14a–S14c deliver the view model, intents, ASCII renderer, the controller over
  the core services, the draft-preserving `$EDITOR` handoff, `pecia tui` with the TTY guard and the
  key-map chapter. The M8 commit carries S14 and the `1.4.0-dev` bump.
- **TUI interaction is line-based.** The loop reads one command per line and redraws the screen; raw
  keypresses and a richer console experience are a follow-up. The CLI remains the complete
  automation surface.
- **Legacy books** under `Assets/Docs` are untouched and unreadable by CLABOK (no import/adopt).
- The daemon never auto-starts; `pecia serve` is explicit. The grammar server (S13) is a separate
  process.
- The cut-over store (`_CLABOK/`) is not tracked. A fresh clone has no library book until the
  runbook in `plans/11-library-docs-and-cutover.md` §6 is replayed.

## 8. Starting the next session

1. Read this file, `README.md` §4/§4.1 and the remaining plan (S01–S14 are implemented).
2. `git status` is clean after the M8 push; `HEAD` is the M8 commit; `Version` is `1.4.0-dev`.
3. Run `go test ./... -count=1` and `go build -o pecia.exe ./cmd/pecia`, then
   `.\pecia.exe sync --check` (must be clean).
4. The planned scope ladder is complete; start new work with a new scope and plan.
