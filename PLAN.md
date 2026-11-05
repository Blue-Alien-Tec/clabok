# CLABOK — Body Granularity, Structural Authority, Documentation Location and Central Store (Conceptual Target Model)

Status: draft (2026-11-04, revised 2026-11-05). Conceptual only: no implementation design, no
migration started. Owner decisions are recorded in §3.1–3.14. This document records the target
picture, the verified current state and the gap between them so the later implementation plan can be
written against it.

Source: the eight-point target model supplied by the project owner (2026-11-04), the follow-up
thoughts that the documentation tree should leave `Assets`, that all working data moves into a single
store, and that temp moves into the project's `.temp` area, and a read-only analysis run through the
tool itself against the live library.

Reference implementation: `F:\UnityProjects\AsteroidBeltInn\Tools\agents\docenginelib` and the
neighbouring tool folders — DocEngine 2.4, book schema v3, container format 1. Read-only reference;
DocEngine itself is not modified.

Implementation target: `F:\UnityProjects\CLABOK`.
Scope breakdown: `Planning/README.md` (planning units; this document stays the normative source).
Language: Go.

Scope: the document model (structural blocks, body granularity, heading ownership), the render
output modes, numbering ownership, the library's structural authority, the hard cut away from the
existing books, the target location for new books, the central working store, the `.temp` area, and
the implementation target `F:\UnityProjects\CLABOK`. Not in scope: any implementation choice, command
syntax, schema version or file format change.

---

## 1. Purpose

The reference implementation (DocEngine) currently supports two opposite granularity regimes without
stating which one is canonical: monolithic chapter bodies of up to 1021 lines on one side, 178
micro-bodies with 40-line sections on the other. The target model fixes a single rule — structure is
data, blocks are the only bodies — and moves all structural authority into the library. A block's
placement becomes a property of the manifest, so moving prose from one place to another no longer
moves or rewrites a body.

This plan defines that target in normative statements, inventories the current divergences and
records the owner decisions taken so far.

## 2. Verified Current State

Everything below was read from the reference implementation's live library, not inferred from
documentation. `check --library .` reports 78 generated files and 0 findings; `book status` reports
six books `clean` and the library book `dirty-input` (the output is byte-identical per `sync
--check`, so only the stored input hash differs).

### 2.1 Library and books

| Entry | Kind | Blocks | Bodies | Storage |
|---|---|---|---|---|
| `library` (repo `README.md`) | library | 17 inline content blocks | 17 | `.lybrary` registry + README |
| `ahadv` | book | 178 | 178 | `_source/ahadv.book` (archive) |
| `concepts` | book | 12 + 1 link | 12 | `concepts.book` (archive) |
| `factions` | book | 17 | 17 | `factions.book` (archive) |
| `engineering` | book | 12 + 3 links | 12 | `engineering.book` (archive) |
| `srr` | book | 3 | 3 | `srr.book` (archive) |
| `docengine_notes` | book | 6 | 6 | `docengine_notes.book` (archive) |

All bodies are ZIP members `blocks/<id>.md` inside the book container. No book stores bodies as
per-section files on disk; `DocEngine_Field_Notes/_source/blocks/` and `_source/layouts/` exist but
are empty.

### 2.2 Body granularity today

| Book | Largest body | Regime |
|---|---|---|
| `concepts` | 968 lines / 72.4 KB | one body per chapter |
| `srr` | 1021 lines / 43.2 KB | one body per chapter |
| `engineering` | 870 lines / 56.2 KB | one body per chapter |
| `factions` | 323 lines / 30.9 KB | one body per chapter |
| `docengine_notes` | 87 lines / 7.1 KB | chapter plus section bodies |
| `ahadv` | 92 lines / 14.9 KB | one body per heading, 101 of 178 under 1 KB, 13 stubbed chapter bodies |

### 2.3 Structural vocabulary today

The manifest uses `kind` values `chapter`, `section`, `content`, `link`, `part`. Kinds are not bound
to a depth:

- `ahadv`: 24 chapters at depth 1, 2 chapters at depth 2, 7 chapters at depth 3, 107 sections at
  depth 2, 3 sections at depth 3, 35 content blocks at depth 4.
- Blocks below depth 1 may still carry a `file` value: 9 do, and none of those files exist
  (`Grossmaechte.md`, `Einflussreiche_Spezies.md`, `Weniger_einflussreiche_Spezies.md`,
  `Unterdrueckte_Spezies.md`, their `-2` variants, `Mentale_Konflikte.md`, `Soziale_Konflikte.md`).
  The renderer ignores `file` below depth 1 and renders those blocks into the parent chapter file.
- `MAX_DEPTH` is 6 (`model.py:71`), so the hierarchy is open-ended rather than three structural
  levels plus blocks.
- Content blocks cannot have children (validated in `model.save`).

### 2.4 Behaviours that the target model touches

| Behaviour | Current implementation | Consequence for the target |
|---|---|---|
| Heading emission | `Numbered.heading` = `'#' * depth + number + title`; emitted by `render_chapter` before the body | Structural headings are already generated, not authored |
| Body heading handling | `shift_headings(body, depth + 1)` rewrites every heading inside a body so the shallowest becomes `depth + 1` | A `####` in prose is treated as structure, not as text |
| Content blocks | `is_content` bodies render verbatim, with no generated heading | In `ahadv` the visible heading *is* `#### Zenari` inside the body; the manifest title is duplicated there |
| `part` grouping | Rendered only by the index (`render.py:_index_lines`): a bold `**<part>**` line when the value changes, or `**<title>**` plus optional body for a `kind: part` node (legacy only). `render_chapter` never emits it | Keep the `part` value as an index-only grouping; the `kind: part` node is dropped |
| ASCII UI catalog | `CATALOG_WIDGETS` is a hardcoded set of project widgets (`UIPanel`, `UIButton`, ...); `WIDGET_FOR_KIND` maps kinds to those names, and `test_asciiui_catalog.py` asserts parity with `Assets/Game/Scripts/Core/UI/Widgets` | Target: generic type names with config overrides; no project parity test |
| Title | Rendered as `# <book.title>` from manifest metadata (`render.py:452`, `render.py:743`); no title body exists | Consistent with the refined target: the title stays data |
| Rename | `rename` sets `block.title` only (`commands_block.py:279-289`) | Renaming an `ahadv` content block leaves the visible `####` heading stale |
| Lint | `doccheck.check_document` flags `heading-jump` and `duplicate-heading` over every heading in the rendered file | Prose `####` is linted as structure |
| Numbering | `numbering.enabled` per book manifest; `format_number` and `anchor_for` embed the number in the heading and in the anchor slug | Toggling numbering changes anchors and therefore refs |
| Numbering command surface | Set at `book new --numbering` and via `apply --plan`; no command toggles it later; the library registry has no numbering field | No command surface for library defaults or book values today |
| Output mode | One Markdown file per chapter plus a generated `README.md` index; only the library book renders as one document (`is_library`). No part-wise mode exists | Target: README always the index; monolith = index plus full text; chapter-wise and part-wise opt-in (part-wise proposed) |
| Structural commands | `add`, `move`, `remove`, `rename`, `set`, `apply`; manifest-level settings (`numbering`, `toc`, `index.depth`, `output`) are reachable only through `apply --plan` or direct `book.json` | Structural authority is command-based for blocks, not for book settings |
| Reorder and reparent | `move` accepts `--before`, `--after`, `--child-of` and `--top`, so any tree node, including chapters and subchapters, can be reordered or reparented today | The capability exists and must survive the split into structure-as-data plus blocks |

### 2.5 Reference-implementation defect

`body list --json` fails with `TypeError: Object of type Block is not JSON serializable`
(`commands_body.py:62`) for books with nested blocks. `model.Book.iter_blocks()` yields
`(block, parent)` (`model.py:273-281`), while `body.list_bodies` stores that second tuple element as
the `depth` field. Flat books report `depth: null` and stay serializable; `ahadv` and
`docengine_notes` crash. The text output of `body list` is unaffected.

This is a reference-implementation defect. DocEngine is not changed; CLABOK must not reproduce it.

### 2.6 Documentation location and `.meta` files

The whole existing documentation tree sits under `Assets/Docs` in the reference project. In CLABOK,
the library book renders at the project root and other new books render into `Docs/`, outside
`Assets`. Per the owner decision of 2026-11-04, the existing tree is not moved.

Verified state:

- `Assets/Docs` contains 6 book folders and 121 `.meta` files, including one `.meta` per book folder,
  one per nested folder (`Factions`, `_source`, `blocks`, `layouts`, `rollback`) and one for
  `Assets/Docs` itself.
- These documents are not referenced from C#: a search over `Assets/Game/**/*.cs` for `Assets/Docs`
  returns 0 matches.
- The library registry stores root-relative paths (`Assets/Docs/AHAdv`,
  `Assets/Docs/Concepts_And_Systems/Factions`, ...). Generated chapter output is written inside each
  book folder; cross-book links and navigation are relative, so a uniform tree move preserves the
  inter-book link graph.
- The repo `README.md` is the library book. Its Document Index links and its repository-layout block
  are derived from registry paths and `git ls-files`, so both would change if the tree moved; the cut
  leaves them unchanged.
- `book move` moves the book directory **and its sibling `.meta`** (`_meta(source)` in
  `books.py:move_book`), updates the registry entry, records the rename and runs a library sync.
  Moving out of `Assets` would therefore drag the book's `.meta` into the non-Asset target unless
  that step changes.
- `book new` defaults to `Assets/Docs/<slug>` and rejects any target outside `Assets/Docs` without
  `--allow-outside` (`commands_book.py:185-192`).
- `flush` scans `root/Assets/Docs/**/_source` for stale inputs (`flush.py:64-66`).
- Help texts and docstrings name `Assets/Docs` in four places (`cli_parser.py:731`, `doccheck.py:8`,
  `docmove.py:18`, `docsplit.py:11`).
- Three test modules use `Assets/Docs` paths (`test_docengine_book.py`, `test_docengine_flush.py`,
  `test_docengine_layout.py`).
- 11 Markdown files inside the documentation tree reference `Assets/Docs` in prose.
- Outside `Assets`, Unity generates no `.meta` files, so a new location needs none. The 121 existing
  `.meta` files stay with the legacy tree; removing them is not part of the cut.

Consequences already visible without a decision: for newly created books, registry paths, tool
defaults and guards, tests and generated links must agree on the new location. The
legacy tree keeps its current paths and therefore keeps its `.meta` and Unity import overhead.
Working data is centralized separately in the store (see §3.2), so the documentation path carries
output only.

### 2.7 Temp folders today (DocEngine scope)

- DocEngine's transient authoring inputs live per book in `_source/.tmp` (present in `ahadv` and
  `docengine_notes`); `flush` scans `Assets/Docs/**/_source` for them (`flush.py:64-66`).
- `.gitignore` ignores `**/_source/.tmp/`, `**/_source/tmp/` and `**/_source/tmp~/`.
- `Assets/.temp` does not exist in the reference project yet.
- Two DocEngine-internal mechanisms stay process-local: `asciiui` materializes archive books into an
  OS temporary directory, and `textsafe` writes sibling temp files before an atomic replace. The
  LanguageTool daemon keeps its state, lock and log under the OS temp directory
  (`grammar_server.py:51-77`, overridable with `DOCENGINE_LT_STATE_DIR`).
- The DocEngine Unity grammar bridge (`Assets/Editor/DocEngine`) spawns the CLI and writes no files
  of its own (verified: that folder contains no file-write call). Its only runtime state is the
  LanguageTool daemon state above.
- Explicitly out of scope: the root `Temp/` files written by the Unity editor tooling
  (`AgentConsoleLogger.cs:16-22`, `AgentDomainReloader.cs:50`). That tooling is not DocEngine, and its
  log, error and trigger files are not part of this plan.

## 3. Target Model (normative, as stated)

Revision (2026-11-04): the target was refined after the first statement. The Title, and every other
structural level, is a value used for rendering; only blocks remain as body files. The earlier
"four body kinds" phrasing is superseded. The statements below are the current target.

1. **Library ownership.** The library manages its books and maintains the repo `README.md` as its own
   monolithic book.
2. **Structure is data.** Title, chapter and subchapter are values (title, kind, position) with no
   body of their own. They exist to be rendered. The structure tree is stored in the book manifest.
3. **Blocks are the only bodies.** Every body file belongs to exactly one block. A block holds prose
   and no structural children of its own.
4. **Placement.** Title, chapter and subchapter hold any number of blocks, rendered in their stored
   order. Every block belongs to one structural node or one region collection and has an ordered
   position there. Region collections are ordered block-ID lists with a stable ID and a target (a
   node, a block or the book). Chapters and subchapters have an order among their siblings. Moving a
   block or a structure node reassigns its placement; the moved unit keeps its identity, and a block
   keeps its body. Reordering and reparenting of chapter and subchapter nodes must stay possible.
5. **Prose discipline.** Prose is written only into blocks. Blocks must not contain `#`, `##` or
   `###`.
6. **Prose headings.** `####` or deeper inside prose is text: it is neither a body unit nor a deeper
   structural level.
7. **Output.** `README.md` is always rendered and always carries the document index. In monolith mode
   the same README holds the index followed by the full book text. In part-wise and chapter-wise
   modes the README holds the index and links into the part or chapter files. Part-wise and
   chapter-wise are opt-in and reversible.
8. **Numbering.** Numbering is opt-in and applies to the structure. The library holds the defaults
   and policy; the effective values are stored in the book manifest.
9. **Structural authority.** Structure is owned by the library and changes only through explicit
   commands.
10. **Index grouping.** A `part` value on a chapter or link is an optional grouping of the generated
    index only. It creates no heading, does not affect numbering and is not a structural level. There
    is no part node and no part body. A part-wise output mode uses the same value as a file boundary
    (see §3.4); it does not become a structural level.

### 3.1 Delivery approach (owner decision, 2026-11-04)

- **Hard cut, no migration of existing books.** The books currently under `Assets/Docs` are not
  migrated, changed, deleted or moved by this plan. The new tool does not read legacy containers; the
  current tool version keeps managing them. In CLABOK, the only book is the library book
  (`README.md`); the legacy books in AsteroidBeltInn remain untouched.
- **Import is paused for the first stage.** The import paths (`import`, `library import`,
  `book adopt`) are not part of the first delivery.
- **One exception: the library book.** The library itself (its book content and registry ownership)
  moves with the new system, but its rendered output stays the repo `README.md` at the project root.
  The library takes no other books with it; the six existing content books stay behind.
- **New books are authored through the CLI.** Create the book, build the structure with the structure
  commands, then insert the prose into blocks.
- **Rationale.** This removes migration and import complexity together with their risks. Content is
  carried over deliberately, and the structure starts clean.
- **Future migrations.** Migration support is introduced later, against the clean model, for changes
  that come after the cut.

### 3.2 Central working-data store (owner decision, 2026-11-04)

- **One store for all working data.** CLABOK keeps its working data in a single folder, `_CLABOK` by
  default; the store root is configurable (§3.11).
- **No `_source` folders in the documentation path.** The docs path carries rendered output only.
- **Contents of the store.** Library registry, per-book sources (manifest and bodies), terms,
  rollback packages and the repository-layout sources. Transient inputs live in the temp area
  (see §3.3).
- **The README stays at the root.** Its rendered output remains the repo `README.md`; its source
  lives in the store.
- **Legacy books are not part of it.** They keep their existing `_source` folders where they are.

### 3.3 Temp area (owner decision, 2026-11-04)

- **Temp lives in `.temp/` at the CLABOK project root by default; the path is configurable
  (§3.11).** The dot prefix keeps it out of normal content and tooling scans.
- **It stays inside the project.** That keeps it in the consuming agent's project, where the agent may
  write without special rights.
- **Scope is CLABOK only.** The area holds CLABOK's transient authoring inputs.
- **It is the exception to the central store.** Durable working data lives in the store; the temp
  area holds transient inputs and is not part of the durable store.
- **Cleanup stays with `flush`.** No additional size limit or cleanup policy in this plan.

### 3.4 Output modes (owner proposal, 2026-11-05)

- **Monolith is the default.** One rendered `README.md` holding the index followed by the full book
  text.
- **Chapter-wise is the current demo shape.** One generated file per chapter plus a generated index.
- **Part-wise is proposed as a third, optional mode.** One generated file per part, using the same
  `part` grouping the index already applies. `README.md` remains the index in every mode; in
  part-wise mode it lists the chapters grouped by part and links into the part files. The mode is
  opt-in and reversible like the chapter-wise mode.
- **Part-wise requires parts.** A book without parts cannot use part-wise output; in part-wise mode
  every chapter carries a `part` value.
- **Part file names are generated.** The name derives deterministically from the part value; generated
  part files are never hand-renamed. Part values are unique within a book, so names cannot collide.
- **A part has no body.** A part file starts with the generated part heading and then the chapters.

### 3.5 Book open/close lifecycle (owner decision, 2026-11-05)

- **Body data stays packed.** Bodies live compressed in the book container while the book is closed.
- **Every body access requires an open book.** Reading or writing a body requires `open`; a closed book
  is not readable. Opening extracts all body data, and the commands work directly on the extracted
  body files.
- **Closing repacks.** A dedicated close command writes the body data back into the container.
- **Multiple books may be open.** `open` and `close` take an explicit book argument and never touch
  another open book. Open is a global book state, not a per-process session: `open` is idempotent, so
  several processes can work on the same open book under per-block locks (see §3.12). A second open
  never creates a second extracted state.
- **No-ops are no-ops.** `open` on an already-open book and `close` on a closed book succeed without
  side effects; errors are reserved for real errors.
- **Extracted data is authoritative while open.** Reads and renders use the extracted body files; the
  container is not consulted until the book is closed.
- **Rollback tiers.** Every mutating command creates an undo-capable rollback package. Undo packages
  are collected separately, capped at 10, and deleted when the book closes. Closing creates a
  book-level rollback point only if the book changed; at most 3 are kept, oldest evicted.
- **Crash state is reported.** If extracted changes are newer than the container, the state is
  reported and the readable content is delivered directly so re-authoring runs through the normal
  pipeline; there is no separate recover/repair path.
- **Open books are informational findings.** `check` and `verify` report an open book as a
  non-blocking `open-book` finding; it does not change the exit code. `book status` shows the open
  state, and `sync --check` remains a drift check.
- **State is tracked, not inferred.** A state file records which books are open, so the code does not
  have to interpret folder contents.
- **Two coordination backends.** The lifecycle runs under either the file backend or the daemon
  backend with identical semantics (see §3.12).
- **Recovery and determinism.** Closing keeps the deterministic container guarantees (fixed member
  order, timestamp, checksums); an interrupted open or close must be detectable and must not corrupt
  the container.

### 3.6 Documentation location (owner decision, 2026-11-05)

- **The library book renders at the project root as `README.md`.**
- **Other new books render into `Docs/` by default; the docs root is configurable (§3.11).**
- Working data lives in the central store; the legacy tree stays under `Assets/Docs`.

### 3.7 Engineering defaults (2026-11-05)

Applied unless the owner overrides them; these are coding decisions, not product questions.

- **Placement storage.** Blocks are a flat pool of `{id, body}`. Structural nodes and region
  collections hold ordered lists of block IDs; blocks carry no placement data. A move edits two lists
  and never touches a block.
- **Sibling order.** List order, no position fields. Stale or concurrent changes are caught by the
  existing book lock and stale check.
- **Move arguments.** `--child-of <node>` appends, `--index N` places explicitly,
  `--before/--after <block-id>` places relatively.
- **Identity.** Structural nodes and region collections have stable IDs like blocks. Block ID `0` is
  reserved as the new-block sentinel; generated IDs are nonzero.
- **Reparenting.** A node may move across levels. It keeps its ID and its block list, and its children
  move with it. A move that would create a fourth structural level is rejected; the children must be
  moved or promoted first.
- **Rendering between blocks.** One blank line; no separator markers.
- **Mode switches.** Generated files a mode no longer produces are pruned by `sync --prune`.
- **Store layout.** The store root is configurable; below it, `<bookId>/`, `rollback/<bookId>/`,
  `<bookId>/layouts/`, the registry and `terms.txt` are fixed.
- **Open state.** `_CLABOK/open.json`, keyed by book ID, recording output path, pid or session and
  timestamp.
- **Extracted bodies.** `_CLABOK/<bookId>/blocks/` while the book is open.
- **Temp inputs.** `<tempRoot>/<bookId>/`; the temp root defaults to `.temp/` at the project root.
- **Rollback.** Rollback packages live in the store; undo packages are removed on close.
- **LanguageTool daemon state.** Stays in the OS temp directory.
- **Open books are immovable.** A book cannot be moved, renamed or unregistered while it is open.
- **Reorder rendering.** Reordering regenerates numbering and all derived anchors and references in
  the same render.
- **References.** Cross-document references target the stable IDs of structure nodes and blocks; the
  visible anchor is derived at render time. IDs are library-managed and unique across all books;
  global uniqueness is a deliberate constraint, not a derived property.
- **Output mode.** Set per book; the chosen mode applies to every chapter in the book.
- **Heading mapping per mode.** Monolith: README `#`, chapter `##`, subchapter `###`. Part file: part
  `#`, chapter `##`, subchapter `###`. Chapter file: chapter `#`, subchapter `##`.
- **Navigation.** Split files keep the current per-file navigation and ToC; the status system is
  covered in §3.14.
- **No pin.** `pin` is removed; the generated index follows the structure order.
- **Numbering is never authored.** Numbers are added at render time when numbering is enabled; titles
  are stored without them. A numeric prefix in an incoming title (`1.`, `1.1`, `IV.`) is stripped
  during normalization, not rejected.
- **Anchors are number-independent.** Heading anchors derive from the title or ID, not from the
  number. Because references target IDs, enabling, disabling or changing numbering never breaks a
  reference.
- **Uniform commands.** `add`, `rename`, `move`, `remove` operate on structure nodes and blocks; the
  node type follows the target parent (child of the book = chapter, child of a chapter = subchapter,
  child of a subchapter = block). There are no per-type commands. `body` covers prose, `book` covers
  lifecycle and settings.
- **Body writes.** A body-writing command always takes a block ID. `0` appends a new block to the
  target chapter or subchapter; a valid block ID overwrites that block's body; an invalid ID is
  rejected. The target is required for `0` and never moves an existing block.
- **Open/close replaces the storage modes.** A closed book is the packed container; an open book is the
  extracted body files. There is no separate storage-mode setting.
- **Settings live with the book.** `numbering`, `toc`, `index.depth` and `output` are stored in the
  book manifest and changed only through commands. The library holds defaults and policy and can
  apply them across books, but does not store per-book values.

### 3.8 Naming (owner decision, 2026-11-05)

- **Product:** CLABOK — Command-Line Authoring for Blocks and Orchestration Kit.
- **CLI:** `pecia`.
- The current implementation keeps its file names until the rename is planned.

### 3.9 Project boundary (owner decision, 2026-11-05)

- **DocEngine is not changed.** It stays exactly as it is.
- **CLABOK is implemented in its own project:** `F:\UnityProjects\CLABOK`.
- **DocEngine is reference only.** `F:\UnityProjects\AsteroidBeltInn\Tools\agents\docenginelib` and the
  neighbouring tool folders are read-only reference resources.
- **The first book is the CLABOK repo `README.md`** — the library book.

### 3.10 Implementation language (owner decision, 2026-11-05)

- **Go.** CLABOK is implemented in Go; the Python reference implementation is not modified.
- **The whole CLI is the tooling surface.** The Unity editor, AI agents, CI and the TUI use the same
  commands; there is no agent-only or editor-only subset. Humans use the TUI for interactive work,
  and the CLI remains available to them.
- **CLI contract.** Tooling-first: structured JSON is the canonical output and the default; `--text`
  renders a human-readable form. Data goes to stdout, diagnostics to stderr. No prompts, colors,
  paging or emoji. Stable error objects, stable exit codes, `--dry-run` for every mutating command,
  and a versioned schema.
- **Single static binary.** No .NET, Node or Python runtime; builds with `CGO_ENABLED=0`.
- **TUI included.** The tool ships its own terminal UI on top of the same core.

### 3.11 Configuration (owner decision, 2026-11-05)

- **One project config file.** `clabok.json` at the project root; JSON because the Go standard library
  covers it without a dependency.
- **Configurable paths.** Store root (default `_CLABOK`), temp root (default `.temp`) and docs root
  (default `Docs`). The library `README.md` stays fixed at the project root and is not configurable.
- **Configurable limits.** Undo rollback packages (default 10) and book rollback points (default 3).
- **Configurable grammar endpoint.** The LanguageTool endpoint; the default is the local daemon in the
  OS temp directory.
- **Configurable coordination.** `concurrency.mode` selects `file` (default) or `daemon`;
  `daemon.socket` overrides the local socket path (see §3.12).
- **ASCII UI type names.** The layout catalog ships generic defaults (`panel`, `button`, ...).
  `asciiui.types` maps each generic name to an optional project-specific name: a blank or absent entry
  keeps the generic default, a non-blank entry overrides it. Defaults never contain project-specific
  names. There is no parity check against the project's C# types.
- **Precedence.** CLI flags override the config file; the config file overrides built-in defaults.
- **Resolution.** Relative paths resolve against the directory of the config file, not the working
  directory.
- **Discovery.** `--project <dir>` or `--config <file>`; without either, search upward from the working
  directory for `clabok.json`. There is no implicit global config.
- **Validation.** Strict loading: unknown keys are rejected, the config version is required, and an
  invalid config fails before any write.
- **Config commands.** `config show` (effective values), `config get <key>`, `config set <key>
  <value>`, `config unset <key>` (restore the default), `config validate` and `config path` (resolved
  file). They emit JSON by default; `--text` renders human output. `set` and `unset` accept
  `--dry-run`. They are project-level and do not require an open book.

### 3.12 Concurrency and locking (owner decision, 2026-11-05)

- **Two interchangeable backends.** `file` uses lock files, atomic writes and stale detection;
  `daemon` uses in-memory mutexes in a single `pecia serve` process. Both provide the same semantics:
  one writer per locked resource (block or node), consistent reader snapshots, no lost writes and
  stale-session detection.
- **Lock granularity.** Body edits lock a single block ID, so two processes can edit different blocks
  of the same structural node concurrently. Manifest changes lock the affected structural node (or the
  book's structure lock): add, remove, rename, move, reparent and reorder.
- **Lock ordering.** Multi-lock operations acquire in a stable order (book, node, block ID) with
  timeouts, so crossing moves cannot deadlock.
- **Reads.** Body reads use an atomic snapshot and need no write lock; a reader never sees a
  half-written block.
- **Close.** Close waits for or rejects in-flight block edits and never repacks a body that is being
  written.
- **Configurable per project.** `concurrency.mode` selects the backend; the default is `file`. CLI
  flags override the config.
- **The core is backend-agnostic.** Book logic depends on a coordinator interface, never on the
  backend.
- **Daemon transport is local-only.** Named pipe on Windows, Unix socket elsewhere; no TCP listener.
- **Daemon lifecycle.** `pecia serve` is a mode of the same binary. In `daemon` mode the CLI expects a
  running daemon and fails with a clear error when it is absent; it does not auto-start one.
- **Mode changes require no open sessions.** Switching the mode while a book is open is rejected.
- **Version match.** CLI and daemon must run the same version; a mismatch is a clear error.
- **No data difference.** The backend never changes book data, IDs, rendering or output.

### 3.13 Code organization (owner guidance, 2026-11-05)

Starting points, not a contract. The layout will move as the code teaches us; the principles are what
we keep.

- **Packages are the responsibility boundary.** Name a package after what it provides, not what it
  contains. Avoid catch-all packages such as `utils`, `helpers`, `common`, `models` or `types`; if one
  appears, it usually marks a missing responsibility.
- **Dependency direction.** `cmd` and `cli` depend on application services; application services
  depend on the domain and the store. The domain should not import the CLI.
- **Thin entry point.** Keep `cmd/pecia/main.go` to flags, wiring and exit codes; business logic
  belongs in `internal/`.
- **Private by default.** Prefer `internal/` until a public API is real.
- **Consumer-side interfaces.** Prefer small interfaces defined where they are consumed; avoid
  interface-per-type and DI containers.
- **File granularity.** As a rule of thumb, one primary type or feature per file, tests beside the
  code, `doc.go` per package, platform code in `_windows.go`/`_unix.go` files with build tags. Split a
  file when it exceeds roughly 300–500 lines or mixes two responsibilities.
- **Starting layout.** A starting point, not a straitjacket. New packages are fine when they carry a
  real responsibility; merges and renames are expected.
  ```
  cmd/pecia/            entry point
  internal/cli/         command dispatch, text and JSON output
  internal/config/      config load, defaults, validation
  internal/model/       book, node, block, IDs
  internal/structure/   add, move, remove, reorder, invariants
  internal/body/        body read and write, heading rule
  internal/store/       containers, open/close, checksums
  internal/coordinator/ file and daemon backends
  internal/render/      monolith, part, chapter, headings, ToC
  internal/refs/        ID references, anchors, resolution
  internal/library/     registry, books, numbering defaults
  internal/apply/       declarative batch plans
  internal/search/      find over manifests and bodies
  internal/diff/        rendered and rollback diffs
  internal/export/      container member export
  internal/temp/        .temp handling, flush
  internal/asciiui/     layout catalog, generic types, overrides
  internal/layout/      repository layout block
  internal/grammar/     LanguageTool client, terms
  ```
- **Enforcement.** `go vet` and `gofmt` are the baseline. Staticcheck or golangci-lint with
  `depguard` are optional aids, and `go list -deps` is a review aid rather than a gate.

### 3.14 Generated content and editorial state (owner decision, 2026-11-05)

- **Status system.** The editorial status system remains and is opt-in per book. The tool owns the
  state vocabulary (states, assignments, legend, badges) and renders it structurally; authors never
  write the status markup.
- **Layout block.** Opt-in and deferred; not part of the first delivery. With git on the host the
  block is generated from `git ls-files`; without git it is a no-op, not an error. A command authors
  it from a path and a depth (how far the tree branches), and endpoints or branches can be excluded
  afterwards. Per-path prose is authored through commands; the generated tree is never hand-edited.
- **Badges.** Badges are generated from repository facts; an explicit message overrides the detector,
  and the generated badge region is never hand-edited.
- **Regions are blocks.** A region is a block with a stable ID in an ordered collection. There are no
  separate region sources or region types; the collection order is the render order.
- **Notes.** A note is a region collection attached to a structural node or a block; it renders
  directly beneath the target first, before the next element. Placement is tool-owned; the text is
  authored through a command.
- **Footer.** The footer is a region collection with exactly one slot per book. It is tool-owned,
  replaceable through a command, and always renders in the book's `README.md` at the bottom, above the
  navigation.
- **Header and preamble are not part of the model.**
- **Index prose.** Each index entry can carry optional prose, authored through a command and rendered
  in the index.
- **Terms and grammar.** The term list and the grammar gate remain tool-owned. Terms change only
  through the confirmation flow.
- **Book status.** Clean/dirty state from input and output hashes remains the drift signal.
- **Rekey.** IDs are stable by default. A rekey command changes an ID and rewrites the references
  across the library; it is the only way to change an ID.
- **Not carried over.** Header and preamble regions, cleanup of legacy nav/index/banner residue,
  migrate and import (paused), and the storage-mode command (replaced by open/close).

## 4. Gap Analysis

| # | Target statement | Current state | Gap |
|---|---|---|---|
| 1 | Library owns books and its README as a monolithic book | Registry with 7 entries; README is a `kind: library` book with 17 inline content blocks; rendered monolithically | None structural. Numbering and output mode are book-level values; the library has no defaults or command surface for them |
| 2 | Structure is data; title, chapter and subchapter have no bodies | Title is metadata; chapters and sections own bodies (up to 1021 lines); kinds are not depth-bound | Structural nodes still carry prose; structure and content are mixed in the same unit |
| 3 | Blocks are the only bodies and can be placed and moved independently | Bodies exist for every structural node; `ahadv` content blocks are leaves; block identity is not the unit of movement | No block-only body store; moving text means moving or rewriting structural bodies |
| 4 | Monolith by default; chapter- and part-wise opt-in | Chapter files are the default for every book; only the library book is monolithic | Part-wise is missing, and the current default is chapter files |
| 5 | Numbering opt-in; library defaults, book-level values | Per-book manifest flag; no runtime toggle command; numbers embedded in headings and anchors | No library defaults, no command surface; numbers embedded in anchors |
| 6 | Prose only in blocks; structure only via commands | Titles live in the manifest and are renamed by command; bodies may contain headings | Bodies carry headings today (`ahadv`); rename does not reach the visible heading |
| 7 | `####`+ is prose, not structure | Renderer shifts body headings; lint validates them as headings; content-block headings *are* body headings | Renderer, lint and ToC contradict statement 7 |
| 8 | Library owns the authority; the tree lives in the book manifest and changes only via commands | Block structure is command-driven; book settings are reachable via `apply --plan` or direct manifest editing | Authority is split; settings have no command surface |

## 5. Areas the Later Plan Must Cover

Conceptual impact areas only; no solution is proposed here.

- **Document model and vocabulary.** Structure as data (title, chapter, subchapter with no bodies);
  blocks as the only body units; `part` is a value, `link` remains, inline `content` becomes a block;
  `file` is legacy.
- **Structural placement model.** Structural nodes hold an ordered list of block IDs; blocks hold no
  placement data. Includes the sibling order of chapters and subchapters, reparenting across levels,
  and the move arguments that express a placement.
- **Body unit boundaries.** What a newly authored block body may start with, and whether the legacy
  `####`-in-body pattern is documented as a legacy exception rather than as a model rule.
- **New-book authoring flow.** Create the book, choose the opt-ins, build the initial structure, then
  add prose. A block ID of `0` appends a new block to the target chapter or subchapter; a valid ID
  overwrites that block; an invalid ID is rejected.
- **Heading ownership.** Structure headings generated from the manifest; prose headings that are not
  structure. This affects rendering, ToC generation, anchor generation and lint.
- **Render output modes.** Monolith by default as a `README.md` that carries the index and then the
  full book text; chapter-wise and part-wise as reversible options whose README stays an index.
  Part-wise requires parts, generated file names and the per-mode heading mapping; split files keep
  their navigation.
- **Numbering.** Library defaults and policy versus book-level values, and the effect on headings,
  number-independent anchors and stable-ID references.
- **Command surface for structure.** Creation, renaming, movement and deletion for every structural
  unit, plus the book-level settings that currently live in `book.json`.
- **Authority boundaries.** Which files are generated and overwritten, which are authored, and how
  `sync --write` continues to protect foreign files.
- **Documentation location.** `Docs/` at the project root for newly created books, the handling of
  the existing tree that stays where it is, tool defaults and guards, generated Document Index and
  layout block, test fixtures and prose references.
- **Central working store.** One store folder for registry, book sources, terms, rollback and layout
  sources; the documentation path carries output only; how a book is identified once its source and
  its output path are decoupled.
- **Configuration.** One project file for store, temp and docs roots, rollback limits, the grammar
  endpoint and the ASCII UI type overrides, with a command surface (`show`, `get`, `set`, `unset`,
  `validate`, `path`), precedence and validation.
- **Temp area.** One `.temp/` folder at the project root for CLABOK's transient inputs: git ignore
  rules, which writers move there, and cleanup via `flush`.
- **Open/close lifecycle.** Which commands require an open book, where the extracted bodies live,
  how the open state is tracked for multiple open books, and how a crash is reported without a repair
  path.
- **Concurrency and locking.** Two interchangeable backends (file locks versus an in-memory daemon),
  mode selection, per-block and per-node lock granularity, stale detection, crash recovery and
  local-only IPC.
- **Cut-over.** The library book moves to the new system; the legacy books remain untouched and are
  not read by the new tool.
- **Tool documentation and tests.** CLABOK needs its own documentation and test suite; the DocEngine
  documents and tests remain reference material, and the C# widget-parity drift test is not carried
  over.

## 6. Invariants to Preserve

Any future implementation keeps these properties, because the rest of the tooling depends on them:

- Deterministic containers: fixed member order, fixed timestamp, per-member SHA-256, byte-identical
  re-save of unchanged books.
- Open/close safety: an interrupted session is detectable, its readable content is delivered, and
  closing never loses a write.
- Both coordination backends expose identical semantics: one writer per locked resource, consistent
  reader snapshots, no lost writes and stale-session detection.
- Two processes can edit different blocks of the same structural node concurrently; structural changes
  serialize per node, and locks are acquired in a stable order.
- The package dependency direction stays acyclic; the domain never imports the CLI.
- CLI output is machine-readable by default, schema-versioned and stable.
- Block ID `0` is reserved for creation and is never a valid stored ID.
- Drift detection between inputs and generated output (`sync --check`, `book status`).
- Rollback packages for every write to an existing book.
- Cross-book references by stable ID, with rendered anchors derived from the target.
- The grammar gate on prose-writing commands.
- `check` / `verify` as read-only acceptance.
- The term list confirmation flow.
- The CLABOK test suite.
- Configuration is validated before any write, and relative paths resolve against the config file.

## 7. Legacy Library Inventory (untouched by this plan)

The existing library stays where it is and is not converted (owner decision, 2026-11-04). The
library book is the single exception: the library moves with the new system while its rendered output
stays `README.md` at the project root, and it takes none of the content books with it. The inventory
below documents what the cut leaves behind. How that content is transferred into the new structure is
out of scope.

- `ahadv`: 178 blocks, 35 content bodies that start with `#### <title>`, 9 blocks below depth 1
  with a `file` value pointing at a non-existent file, 7 `chapter` blocks at depth 3 and 2 at
  depth 2, 3 `section` blocks at depth 3, 13 stubbed chapter bodies (12 at 1 byte, one empty),
  24 generated chapter files.
- `docengine_notes`: 3 section blocks under one chapter, 3 generated files.
- `concepts`, `factions`, `engineering`, `srr`: flat chapter lists, one body per chapter, one link
  block each in `concepts` and three in `engineering`.
- `library`: 17 inline content blocks rendered into the repo README, plus generated layout, badges
  and index regions.
- Body ownership today: 193 of the 245 bodies belong to structural nodes (chapter and section
  bodies); the 35 `ahadv` content bodies and the 17 library inline bodies are already blocks. This
  imbalance is the main reason the new model starts from a clean book rather than a conversion.
- Temp today (DocEngine only): per-book `_source/.tmp` in two books, the `_source` ignore rules and
  the LanguageTool daemon state in the OS temp directory; `Assets/.temp` does not exist. The root
  `Temp/` files belong to the Unity editor tooling and are not part of this plan.
- Documentation tree: `Assets/Docs` with 121 `.meta` files, 6 book folders, 11 documents that name
  `Assets/Docs` in prose, 4 tool help/docstring sites and 3 test modules.
- Working store: the six content books keep their `_source` folders (6 directories, each with
  `.docengine`, `rollback` and a `.book` container; two also with `.tmp`, one with `blocks` and
  `layouts`). The new system does not reuse them.
- Total: 245 bodies, 78 generated files, 0 current findings.

## 8. Out of Scope

- Detailed command syntax, schema version or file format; the package layout is recorded in §3.13.
- Changes to DocEngine itself; it is a read-only reference for this plan.
- Import, adopt and migration of the existing books (owner decision: hard cut, import paused),
  including conversion, content mapping and re-authoring.
- Changing, deleting or moving the legacy books.
- Custom agents, their rules and their configuration.
- Any change to the SRR side of the project or to the Unity code base.
- Any change to the grammar, term-list or layout subsystems beyond the behavior stated in this plan.

---

## 9. Open Questions and Decision Points

No open questions remain. Owner decisions are recorded in §3.1–3.10.
