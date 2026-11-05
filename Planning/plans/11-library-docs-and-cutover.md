# S11 Implementation Plan — Library Book, Documentation Location and Cut-over

Status: planned (2026-11-05)
Scope: [../11-library-docs-and-cutover.md](../11-library-docs-and-cutover.md)
Normative source: [../../PLAN.md](../../PLAN.md) §3.1, §3.6, §3.7, §3.8, §3.9, §3.14.

## 0. Guiding principles

- Flexibility, performance and robustness decide the open points.
- A real error is reserved for a real failure (IO, corruption, invalid data). A condition the user
  can act on is an informative reject with the facts in `details`.
- Reads never write. Rendering never mutates the store or the output when checking.
- One render per book: the same book renders byte-identically whether it is open or closed.
- No silent file moves or deletions; no import, adopt or migration of legacy content.

## 1. Resolved design decisions

### D1 — Registry schema v2

`store.RegistryVersion = 2`. The document becomes:

```json
{
  "version": 2,
  "books": [{ "id": "…", "path": "Docs/…", "kind": "library|book" }],
  "defaults": { "numbering": { "enabled": false }, "toc": { "enabled": false }, "indexDepth": 2, "output": "monolith" }
}
```

- `defaults` is optional (`*model.Settings`); absent means "use the built-in defaults".
- Version 1 loads unchanged (no defaults) and is written back as version 2. A version newer than 2
  is rejected with `store.registry-version`.
- `RegistryEntry` stays `{id, path, kind}`. Book state (open/closed) is **not** stored: it is derived
  from `open.json` at report time, so the registry cannot go stale.
- Validation additions: IDs unique; at most one `library` entry; that entry has path `.`; paths are
  unique, relative, clean and contain no `..` element or leading separator (the registry never
  escapes the project); kind is `library` or `book`.
- Serialization order: the library entry first, then the remaining entries in their current order.
  That order is stable and is the Document Index order.

### D2 — Library defaults

Defaults live in the registry, the library's own document. `library defaults show|set|unset` edits
them; `model.Settings.Normalized()` fills index depth and output before the value is stored.

Defaults are applied **at book creation** only. A book stores its effective settings in its own
manifest (PLAN §3.7: settings live with the book). There is no render-time precedence chain: a
book keeps rendering deterministically from its own manifest, and a later default change cannot
silently re-render old books. `book new` copies the library defaults into the new manifest; explicit
flags at creation and a later `book set` win.

### D3 — Identity and output path

- **Book ID**: `--id`, otherwise a slug of the title (lowercase ASCII, `-` separated, fallback
  `book`). A collision appends `-2`, `-3`, … The ID is stable forever and is never re-derived from
  the title. `book rename` does not change it.
- **Output path** is the registry `path`, a project-relative directory.
- **Default**: `<docs root>/<id>` converted to a project-relative path. The docs root comes from
  `config.Values.DocsRoot` (absolute). If it lies outside the project, creation is rejected with a
  hint to set `--path` or change `docs.root`; the registry never stores an escaping path.
- **`--path`** is project-relative. **`--allow-outside`** permits a path outside the docs root but
  still inside the project. There is no silent third location.
- **One folder per book**, generated files directly inside it (no per-mode subfolder).
- **Rename is title only**; **move is path only**. Neither touches the output directory or the
  container bodies. `library sync --write` renders the new location. Old generated files are not
  deleted (no deletion without an explicit command).

### D4 — `book new`

Creates the container from `model.NewManifest` (random allocator, root node ID returned), applies
the effective settings, saves it closed, registers it, then renders it once with `Sync{Write: true}`.

```
pecia book new --title <t> [--id <id>] [--numbering[=bool]] [--toc[=bool]]
               [--index-depth N] [--output monolith|chapter|part]
               [--path <dir>] [--allow-outside] [--library] [--force]
```

- Bool flags distinguish "absent" from "false" with `flag.Visit`; absent means "library default".
- `--library` forces kind `library` and path `.`, and rejects `--path`/`--allow-outside`. Only one
  library entry may exist.
- A non-empty output directory is rejected (`library.path-exists`) unless `--force`; `--force` is
  also forwarded to the initial render, which is how the scaffold README is replaced at cut-over.
- `--dry-run` validates and reports the planned ID, path and settings, and writes nothing.
- The payload includes the root node ID so the authoring flow can place root blocks.

### D5 — Library book

The library book is the registry entry `{kind: "library", path: "."}`. Its monolith render is the
project `README.md`. It is created with `book new --library --id <id> --title <t>`. It cannot be
moved or unregistered; its title may be renamed like any other book.

### D6 — Document Index

- Generated from the registry in registry order, excluding the library entry itself.
- Rendered only in the library book's `README.md`, as `## Document Index` after the book's own
  `## Index` section (all output modes; in chapter/part modes the README is the index).
- Format: a table `| Book |` with `| [Title](path/README.md) |`. S12 extends it with a note column.
- Titles are read from the book manifests (open extract or closed container member). A registry
  entry without a readable manifest is a real error naming the book. The registry stores no title,
  index file name or note; the manifest and the fixed `README.md` are the only sources.
- The catalog is injected into the render command (`render.CatalogFunc`, implemented by
  `library.IndexEntries`) so `sync --book <library>`, bare `sync` and `library sync` all produce the
  same bytes. The catalog participates in the library book's input hash, so a registry change makes
  the library README dirty.

### D7 — Sync

- `sync --book <id>` renders one book; `sync` without `--book` and `library sync` render every
  registered book and report per book.
- The renderer reads the extracted files when the book is extracted (open or unsaved) and the
  container otherwise. Syncing never opens or closes a book. This is the sole closed read path
  (`PLAN.md` §3.5): body and structure mutation keep the `open` requirement.
- Library-wide sync builds the cross-book layout map from the registered manifests (no bodies, no
  open) and resolves `[[id:N]]` and link entries in another book to a relative href.
- `--check` never creates the output directory.
- An explicit `--check` without `--dry-run` fails with `render.drift` when any book has drift
  (changed, foreign or prunable files), with the full per-book payload in `details`. The implicit
  dry run and `--dry-run` keep the S09 informational behaviour.
- A real error (missing or corrupt container, invalid registry) fails the command and returns every
  collected per-book result in `details`; the remaining books are still assessed.
- Library-wide sync renders the non-library books first and the library book last, so the generated
  index reflects the final state. Registry order determines the index rows, not render order.
- Foreign and prune detection matches the generated marker's book line, not only its first line, so
  a shared directory cannot cross-prune another book's output. No whole-directory ownership scan:
  files the book never produced are never reported or touched.
- Input and output hashes are computed from manifest + bodies in both states, so open and closed
  books compare identically.
- `check`/`verify` report `render.open-book` only when the book is actually extracted (fixes the
  unconditional finding).

### D8 — Move, rename, unregister

- All three require a closed book (`lifecycle.RequireClosed`).
- Registry mutations hold the store lock (`Lifecycle.LockStore`); `book rename` holds the book lock
  and rewrites only the manifest title inside the closed container.
- `book unregister` removes the registry entry and deletes nothing; the receipt names the retained
  output path. The library entry cannot be unregistered.
- None of the three touches the output directory.

### D9 — IDs are immutable; no rekey

Owner decision (2026-11-05): an ID is identity and never changes. Placement is a collection edit:
`move` and reparenting change the ordered lists, the unit keeps its ID, and every reference stays
valid. A copy creates a new ID for the copy and maps the copy's internal references; an import maps
imported IDs to fresh library IDs. There is **no** `rekey` and no `library rekey`. Duplicate
ownership is corruption to report, never to repair by renaming; `sync --check` rejects it with the
ID and the affected books.

### D10 — Lifecycle lock helpers

Add `Lifecycle.LockStore()` (store exclusive) and `Lifecycle.LockBooks(ids)` (book exclusive, sorted
by the coordinator's order). Existing lifecycles keep their lock order: store → book → node → block.

### D11 — Package and wiring

- New `internal/library`: registry-backed defaults, book creation/move/rename/unregister, index
  entries, library sync. It imports `render`; `render` only consumes the consumer-side
  `CatalogFunc`.
- `cmd/pecia` wires `render.Register(registry, library.IndexEntries)` and `library.Register`.
- `internal/library/doc.go` states the responsibility.

### D12 — Cut-over

The scaffold README text is deliberately re-authored as library-book content: description, CLI line
and status become root blocks; "Checks" becomes a chapter with its script and CI note blocks. No
Planning/PLAN content moves. The first render uses `--force` because the scaffold has no marker;
after that `library sync --check` is the acceptance gate. `_CLABOK/` stays untracked; the generated
`README.md` is tracked.

### D13 — Library ID authority

PLAN.md §3.12: "IDs are library-managed and unique across all books; global uniqueness is a
deliberate constraint, not a derived property." The first S11 implementation used a per-book random
allocator, which made uniqueness probabilistic and contradicted that decision. S11 therefore adds
`internal/idspace`:

- The library owns one persisted monotonic ledger (`<store>/ids.json`), guarded by the dedicated
  `KindIDs` lock. Every ID issued by the tool comes from `Allocator.Next`.
- `Allocator.Peek()` previews the next ID without writing; dry runs use it.
- A store that predates the ledger is initialized once by scanning the highest owned ID of every
  registered book; the ledger is then authoritative.
- Lock discipline: the allocator acquires only `KindIDs` and never a store or book lock, so it is
  safe to call while a book lock is held. The ID-lock holder never waits for a book.
- `structure.Add`, `structure.AddBlock`, `apply`, `body set 0` and `book new` use the library
  allocator; the model's `Allocator` interface already anticipated this.
- IDs are immutable (D9). `move`/reparent change only the ordered collections. A copy gets a new ID,
  and duplicate ownership is a `sync --check` finding (`library.id-duplicate`), never a rename.

## 1a. Reference review (DocEngine, read-only)

The reference does several things right and is structurally weak in others. S11 adopts the first
list and deliberately avoids the second.

Adopted:

- Registry validation: unique IDs, unique paths, exactly one library entry.
- Deterministic registry document order: the library entry sorts first, the other entries keep
  their relative order. That order is the Document Index order.
- Index links target the book's index file (`<path>/README.md`), never a hardcoded directory.
- Per-book sync results: a real failure is reported per book and does not abort the remaining books;
  drift, foreign and orphaned files are findings, not crashes.
- Write only after a read-only evaluation pass; `--check` never writes.
- The library context participates in the book's dirty state: adding, moving or unregistering a
  book makes the library README dirty even though its manifest did not change.

Avoided weaknesses:

- **Two sources for the index.** DocEngine stores the Document Index as link blocks in the library
  book *and* reconciles them with the registry on every register/move/unregister. S11 derives the
  index from the registry at render time; it is never stored, so it cannot drift.
- **Denormalized registry metadata.** DocEngine stores `title`, `index` and `note` in the registry
  and silently falls back when a manifest is unreadable. S11 keeps the manifest as the only title
  source, keeps the index file name fixed (`README.md`), and treats an unreadable manifest as a real
  error naming the book.
- **Swallowed errors.** No `try/except: return` around book loads; no fallback title.
- **Whole-directory ownership scan.** DocEngine scans every `*.md` in the output directory and
  reports "unmanaged" files, which is fragile at the repo root. S11 only inspects rendered targets
  and files whose generated marker names the book.
- **Load-everything workspace.** S11 reads one book at a time; only the registry and the short title
  read for the index are shared.

## 2. Command surface

| Command | Mutating | Notes |
|---|---|---|
| `book new` | yes | creates, registers, renders |
| `book move` | yes | registry path only, closed only |
| `book rename` | yes | manifest title only, closed only |
| `book unregister` | yes | registry entry only, deletes nothing |
| `library defaults show` | no | effective defaults and source (one `library defaults` command, `show`/`set`/`unset` action) |
| `library defaults set <key> <value>` | yes | registry defaults |
| `library defaults unset` | yes | back to built-in defaults |
| `library sync` | yes/check | alias of library-wide sync |
| `sync` (no `--book`) | yes/check | library-wide |

## 3. Tests

Scenes use `t.TempDir`, the store/container helpers and `cli.Run` for command tests.

- **store**: v2 round-trip, v1 upgrade, future-version reject, duplicate IDs, duplicate paths, one
  library, library path `.`, path escape rejection.
- **idspace**: sequential and persistent ledger, initialization from existing books, `Peek` without
  a write, version rejection, missing book container, uniqueness across allocator instances.
- **render**: open/closed identical files and hashes; closed render without extraction; conditional
  `open-book` finding; `--check` creates no directory; book-scoped foreign/prune; Document Index
  with two books in registry order; drift gate (explicit `--check` fails, dry run does not).
- **library**: slug and collision; default application and flag precedence; docs root containment
  and `--allow-outside`; non-empty path guard; library singleton; rename/move/unregister closed-only
  and no file movement; index order and links; move keeps the ID and the reference; duplicate
  ownership rejected by `sync --check`; library sync with two books and per-book drift.
- **CLI**: every new command through `cli.Run`, including `--dry-run` receipts and reject exit 3.

## 4. Acceptance mapping

| Scope criterion | Covered by |
|---|---|
| 1 new book under docs root and rendered | D4, `library` tests |
| 2 explicit outside flag | D3, D4, path tests |
| 3 library README index in registry order | D6, render index test |
| 4 defaults vs per-book value | D2, `book new` precedence tests |
| 5 library sync drift per book | D7, sync tests |
| 6 move/rename registry and closed guard | D8, command tests |
| 7 legacy containers ignored | no import path; registry-driven tests |
| 8 cut-over clean | D12 runbook |
| 9 no import/adopt command | command inventory review |

## 5. Verification

`gofmt -l .` empty, `go vet ./...`, `go build ./...`, `go test ./... -count=1`; then the cut-over
runbook and `pecia sync --check` on the repository. No commit, no push, no version bump without the
milestone-close decision.

## 6. Cut-over runbook

The library book is the repo README. The scaffold text is re-authored through the CLI; nothing is
copied by a migration command. `_CLABOK/` is untracked, so a fresh clone recreates it with the same
steps (IDs are generated per run; read them from the command payloads).

1. `pecia book new --library --id library --title CLABOK --force`
   - `data.root` is the title node ID. `--force` replaces the hand-written scaffold README once.
2. `pecia book open library`
3. Three root blocks under `data.root`:
   - "Command-Line Authoring for Blocks and Orchestration Kit."
   - "CLI: `pecia`"
   - "Status: initial Go scaffold. The full documentation is authored with CLABOK itself; this
     README is the first book."
   Each with `pecia body set --book library 0 --parent <root> --file <file>`.
4. `pecia add --book library --title Checks`; `data.target` is the chapter ID.
5. Four blocks under the chapter: "Local:", the PowerShell fence, the sh fence, the
   `release-ci.yml` line. `## Checks` is generated from the chapter title.
6. `pecia book close library`
7. `pecia sync --write`
8. `pecia sync --check` → exit 0, `"clean": true`.

What the scaffold keeps: all of its text. What is generated: `# CLABOK` (manifest title),
`## Index`, the `## Document Index` when other books are registered, and `## Checks`. The legacy
books and the Planning/PLAN documents are not touched.

The cut-over was executed on 2026-11-05: the store is at `_CLABOK/` (gitignored) and the generated
`README.md` is tracked. `pecia sync --check` reports `clean: true`.
