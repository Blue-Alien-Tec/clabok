# S09 — Rendering and Output Modes

Status: implemented (2026-11-05). Delivery slices: S09a (render modes, index, sync, check) and S09b (ASCII UI catalog, anchor rendering). The index-prose field arrives with S12.
Phase: 5. Depends on: S04 (store), S07 (structure, settings), S08 (bodies). Blocks: S10, S11, S12, S14.
PLAN refs: §3.4 (modes), §3.7 (rendering between blocks, mode switches, heading mapping, navigation,
no pin), §3 (7), §3.14 (index prose rendering), §6 (deterministic output, drift detection, `check`).

## 1. Goal

Turn a book into deterministic Markdown in one of three modes, with generated structural headings,
a generated index in the README, per-file navigation and pruning of files a mode no longer produces.

## 2. In scope

| # | Requirement | PLAN |
|---|---|---|
| 1 | `README.md` is always rendered and always carries the document index | §3 (7), §3.4 |
| 2 | Monolith is the default: one rendered `README.md` with the index followed by the full book text | §3.4 |
| 3 | Chapter-wise: one generated file per chapter plus a generated index in the README | §3.4 |
| 4 | Part-wise (opt-in, reversible): one generated file per part, using the existing `part` grouping; README lists chapters grouped by part and links into the part files; a book without parts cannot use it; every chapter carries a `part` value in that mode | §3.4 |
| 5 | Part file names derive deterministically from the part value; generated files are never hand-renamed; part values are unique within a book | §3.4 |
| 6 | Region collections present in the model render at their target position: the book footer slot, directly beneath a node or block for notes, and on the entry for index prose. The placement contract lives here; authoring, status vocabulary and badge detection live in S12 | §3.14 |
| 7 | Heading mapping per mode: monolith README `#`/`##`/`###`; part file `#`/`##`/`###`; chapter file `#`/`##` | §3.7 |
| 8 | Structural headings are generated from the manifest and never authored; a prose `####` or deeper is rendered verbatim as body text and never enters the ToC or the structural hierarchy | §3 (6), §3.7 |
| 9 | Index generation: entries in structure order, `part` as an index-only bold grouping line (no heading, no numbering effect), optional index prose per entry, no pin | §3 (10), §3.14, §3.7 |
| 10 | Rendering between blocks: one blank line, no separator markers | §3.7 |
| 11 | Split files keep per-file navigation and ToC: previous/index/next at the top and bottom with `---` separators, previous/next omitted at boundaries, `**Contents**` before the H1 when `toc` is enabled. The README carries the Document Index link (library) or the Library link plus the first-document link in split modes | §3.7, §3.14 |
| 12 | Mode switching is reversible; `sync --prune` removes generated files a mode no longer produces | §3.7, §3.4 |
| 13 | `sync --check` is a read-only drift check; `sync --write` writes only changed files and refuses to overwrite a file without the generated marker unless forced | §3.14, §6 |
| 14 | Render is a pure function of model, bodies and settings: same input, byte-identical output | §6 |
| 15 | `check` and `verify` are read-only acceptance over the rendered result, including structural lint that ignores prose headings | §6, §3 (6) |
| 16 | ASCII UI catalog: generic type names (`panel`, `button`, ...) as defaults, `asciiui.types` overrides from S02, rendered ASCII spliced between markers; no parity check against project C# types | §3.7, §3.11 |
| 17 | Output mode is set per book and applies to every chapter of the book | §3.7 |
| 18 | Render reads a closed book's packed container directly, read-only and without extracting; an open book renders from its authoritative extracted state. Modifying commands keep the `open` requirement | §3.5 |

## 3. Out of scope

- Numbering values, number formats, anchors and reference resolution: S10 (the renderer calls into
  it; this scope does not decide anchor semantics).
- Authoring of footer, notes, status, badges, layout and index prose: S12.
- Library-wide sync across registered books and the README as the library book: S11.
- Container codec and atomic writes: S04.
- Grammar findings: S13.
- Model and command surface: S03, S07.

## 4. Deliverables

- `internal/render`: render pipeline, mode selection, heading emission, ToC, index, navigation,
  pruning, generated-file markers, output hashes.
- `internal/asciiui`: catalog, generic defaults, override resolution, block splicing.
- `internal/cli` command group for `sync` (`--check`, `--write`, `--prune`, `--force`) and `check` /
  `verify`.
- Golden-file fixtures: one small book rendered in all three modes; a book with notes, footer and
  index prose; a book with code fences and `####` prose.
- Tests: mode matrix, heading mapping matrix, prune matrix, drift detection, foreign-file protection,
  byte-identical re-render.
- This scope's implementation plan under `Planning/plans/`.

## 5. Acceptance criteria

1. Rendering the same fixture twice yields byte-identical files (hash comparison).
2. Monolith output contains the index followed by the full text; chapter-wise output writes one file
   per chapter plus the index README; part-wise output writes one file per part plus the index README.
3. Part-wise on a book without parts fails with a stable code; part-wise with two chapters sharing a
   part value produces one part file; duplicate part names are rejected.
4. The heading mapping matches the mode matrix exactly, including subchapters.
5. A `####` line in a body renders verbatim and appears in no ToC and no anchor table.
6. `sync --check` reports drift without writing; `sync --write` writes only changed files; a foreign
   file without the generated marker is not overwritten without `--force`.
7. Switching monolith -> chapter-wise -> monolith with `--prune` leaves exactly the generated files
   of the active mode.
8. Navigation in split files links the correct previous, index and next targets, omits the absent
   direction at a boundary, and places the footer above the bottom navigation; the README carries
   the Document Index link (library) or the Library and first-document links (other books).
9. `check` reports nothing for a clean fixture and reports a stable finding code for an injected
   structural defect; `verify` does not write.
10. `asciiui` renders identical output for the same spec with defaults, and applies an override from
    `asciiui.types`; no test references project C# types.
11. Index prose renders on its entry; an entry without prose renders unchanged.

## 6. Design decisions for this scope's planning session

- Generated file naming for chapters and parts: slug rules, transliteration, collision handling, and
  whether the number is part of the file name (`PLAN.md` forbids numbers in titles, not in names).
- The generated marker/banner format and its role in foreign-file protection.
- Where output hashes live (S04 hash pair) and what exactly is hashed.
- Index line format per mode, including the part grouping line and the depth limit (`index.depth`).
- ToC depth per mode and whether a `toc` setting can be `false`.
- Resolved (2026-11-05): navigation is visually identical to DocEngine. The labels are `← Previous`,
  `Index`, `Next →`, `← Library`, `First document →` and `Document Index`; split-file content is
  wrapped in `---` rules; the README carries top and bottom navigation derived from the library
  context and the footer sits above the bottom navigation. `toc` applies to split files; the
  monolith keeps the index.
- How the README title is derived (book metadata only; S03 stores it).
- Whether rendering a book into `Docs/<slug>/` is a store responsibility or a render responsibility;
  decide the seam with S11.
- ASCII UI spec syntax and marker syntax, and whether the catalog is data or code.
- What `check` reports versus what `verify` reports (overlap and difference).
- Resolved (2026-11-05): render reads a closed container read-only and never opens the book; the
  `open` requirement stays with modifying commands (`PLAN.md` §3.5). `check` and `verify` use the
  same read-only source.

## 7. Risks and constraints

- Output modes must not change the model. A mode is a render parameter, never a storage decision.
- Prose headings are text. Any code path that shifts, renumbers or indexes them violates `PLAN.md`
  §3 (6); the reference implementation does exactly that (`shift_headings`), so do not copy it.
- Determinism: map iteration, locale-dependent sorting and `time.Now()` are the usual leaks.
- The ASCII UI parity test from the reference implementation is explicitly not carried over.
- `sync --write` protecting foreign files is a safety property; test it with a hand-written file.
