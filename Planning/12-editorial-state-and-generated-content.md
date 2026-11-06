# S12 — Editorial State and Generated Content

Status: in progress (S12a delivered 2026-11-05; block-reference correction and index table
2026-11-06; S12b, S12c and S12d delivered 2026-11-06, with S12d as the last slice of this segment;
S12e deferred to the next planning segment as its first task).
Phase: 7. Depends on: S09 (render placement contract). Blocks: none, but S11 is its first consumer.
PLAN refs: §3.14 (regions, footer, notes, index prose, status, badges, layout block), §3 (4), §3 (5),
§3.11 (asciiui types are an S09 concern).

## 0. Current state and pre-planned decisions (2026-11-05)

- Regions already exist in the model (`Region{ID, Kind, Target, Blocks}`; footer/notes) and the
  renderer renders them (`footer`, `notesFor`) at their target. Authoring commands are missing.
- `internal/asciiui` exists: `Parse`, `Render(spec, overrides)`, `Defaults`, `Resolve`. The renderer
  reads `<book>/layouts/asciiui/<blockID>.asciiui` and reports `render.missing-layout` when absent.
- `structure.AddLockedBlock(book, parent, "UILayout")` creates the locked anchor block; the body and
  type commands reject locked blocks, while moves and removals stay allowed.
- Index prose is not modelled; `render`'s `entry`/`indexMarkdown` is the render hook.
- Status, badges and the layout block do not exist.

Pre-planned decisions:

- Manifest v3 adds an optional index-prose **block reference** per node (`proseBlock`) and any status
  fields; one version bump covers all S12 model additions. The prose itself is a block: `PLAN.md` §3
  statement 5 says prose is written only into blocks, so the node carries the reference and never the
  text.
- The delivered S12a node text field `Node.Prose` was corrected on 2026-11-06: it contradicted
  `PLAN.md` §3 statement 5. The text now lives in a block referenced by `proseBlock` and owned by a
  `prose` region; there is no second prose form.
- ASCII UI commands: `asciiui add --book <id> --parent <node> --file <spec>`,
  `asciiui set --book <id> --block <id> --file <spec>`, `asciiui remove --book <id> --block <id>`
  (delete the spec, then `remove` the locked block). The spec path is fixed, not configurable.
  Delivered 2026-11-06: the spec and the block are written in one atomic step, and the lifecycle
  packs the specs on close and extracts them on open.
- Region commands: `region set --book <id> --kind footer --file <path|->`,
  `region notes --book <id> --target <node|block> --file <path|->`,
  `region remove --book <id> --target <node|block>`; the footer slot is one per book.
- The index shape is a derived state, not a setting: a book without index prose and without status
  renders the plain list; prose or status makes it the `Document | Description` table. The per-file
  ToC stays a list and never carries prose.
- Status is opt-in per book, tool-owned vocabulary, rendered through the placement contract.
  Delivered 2026-11-06: the fixed vocabulary, the per-node/block assignment, the index status column
  and the legend; a disabled book renders nothing and keeps its assignments.
- Badges and the layout block stay last; the layout block is deferred and uses `git ls-files`.

## 1. Goal

Model authored content that is not part of the chapter flow — footer, notes and status — as blocks in
ordered collections, and generate the repository-derived content (badges, layout tree) from facts
instead of hand-written markup, while removing the header and preamble regions from the model.

## 2. In scope

| # | Requirement | PLAN |
|---|---|---|
| 1 | Regions are blocks: a region is a block with a stable ID in an ordered collection; there are no separate region sources or region types; collection order is render order | §3.14 |
| 2 | Region collections have a stable ID and a target: a node, a block or the book | §3 (4), §3.14 |
| 3 | Footer: a region collection with exactly one slot per book; tool-owned, replaceable through a command, always rendered in the book's `README.md` at the bottom above the navigation | §3.14 |
| 4 | Notes: a region collection attached to a structural node or a block; rendered directly beneath the target, before the next element; placement tool-owned, text authored through a command | §3.14 |
| 5 | Header and preamble are not part of the model and have no code path | §3.14 |
| 6 | Status system: opt-in per book; the tool owns the vocabulary (states, assignments, legend, badges) and renders it structurally; authors never write the status markup | §3.14 |
| 7 | Badges: generated from repository facts; an explicit message overrides the detector; the generated badge region is never hand-edited | §3.14 |
| 8 | Layout block: opt-in, deferred from the first delivery; generated from `git ls-files`; without git it is a no-op, not an error; a command authors it from a path and a depth; endpoints and branches can be excluded afterwards; per-path prose is authored through commands; the generated tree is never hand-edited | §3.14 |
| 9 | Index prose: each index entry can carry an optional block reference (`proseBlock`); the prose is a block in the block pool, authored through a command and rendered by S09 | §3.14, §3 (5) |
| 10 | Every authored change goes through a command with dry-run, rollback and open-book requirement | §6 |
| 11 | Removal of region content is a command; there is no hand editing of a generated file | §3.14 |

## 3. Out of scope

- Render placement mechanics, heading emission, index line format: S09.
- Grammar gate and term list: S13.
- Status or badge authoring on the rendering side beyond the placement contract: S09.
- Header, preamble and legacy residue cleanup: explicitly not carried over.
- Layout block as part of the first delivery: explicitly deferred by `PLAN.md` (this scope delivers
  it last, and can ship without it).

## 4. Deliverables

- `internal/model` extension for region collections where S03 did not already cover it (coordinate:
  S03 owns the type, this scope owns the semantics and commands).
- `internal/editorial` (or the responsible package): footer, notes, status, badges, layout, index
  prose authoring and storage.
- Commands for each region and for status assignment/legend and badge override.
- Badge detectors driven by repository facts (git remote, license, Go version, module path).
- Layout generator over `git ls-files` with depth, include/exclude and per-path prose.
- Tests: footer single-slot enforcement, note adjacency after a reorder, status legend golden, badge
  override precedence, layout with a fixture repository and without git, exclusion rules.
- This scope's implementation plan under `Planning/plans/`.

## 5. Acceptance criteria

1. A footer command replaces the single slot; a second footer is impossible; the footer renders in
   the README above the navigation in every output mode.
2. A note attached to a node renders directly beneath that node, before the next element, and follows
   the node after a reorder.
3. Removing a note removes exactly its block and leaves the rest of the collection order intact.
4. Status is opt-in: a book without status renders no legend and no status markup; an enabled book
   renders states, assignments and legend from tool-owned data.
5. An explicit badge message overrides the detector; a detector without a source omits the badge and
   reports it, and no badge line is hand-editable (a hand edit is not preserved by a render).
6. The layout block renders a deterministic tree from a fixture repository, honours depth and
   exclusions, and is a no-op without git.
7. `grep` over the code base finds no header or preamble region code path.
8. The index-prose reference round-trips through its command and renders on its entry (contract test
   with S09); the prose text is a block, and no node text field for prose exists in the schema.
9. Every change in this scope creates a rollback package and respects the open-book requirement.

## 6. Design decisions for this scope's planning session

- Status vocabulary and its transitions (which states exist, whether they are configurable per book).
- Badge detector set and the repository facts each one reads; what "optional" means for a missing
  source (omit versus placeholder).
- Whether the layout block, explicitly deferred, is part of this scope at all or a separate follow-up
  scope. If it stays, plan it as the last milestone of S12.
- Note ordering when several notes target the same node, and whether a note can target a region.
- Footer command shape (set, clear, show) and whether the footer can contain multiple blocks (one slot
  can still hold an ordered list).
- Settled by `PLAN.md` §3 statement 5: index prose is a block, and the node carries only the
  reference. Do not reopen this as a field-versus-block question.
- Where the generated marker for badges and layout lives, and how it interacts with S09's
  foreign-file protection.
- Whether status is rendered as inline markers, a trailing legend, or both, and the exact rendering
  belongs to S09's placement contract.

## 7. Risks and constraints

- Generated content must regenerate deterministically. A badge or layout block that depends on the
  environment (locale, time) breaks `sync --check`.
- `git ls-files` only lists tracked paths. Untracked files never appear; that is intended and must be
  documented in the tool output.
- The footer and notes are blocks. Do not introduce a second region storage next to the block pool;
  `PLAN.md` removed the separate region sources deliberately.
- Index prose is a block reference, never a node text field. A text field for rendered prose breaks
  `PLAN.md` §3 statement 5 and reintroduces a prose path without block ID, per-block lock and
  rollback.
- Header and preamble must be gone, not deprecated. Grep for them in the acceptance test.

## Refinement (2026-11-05)

- The ASCII UI layout feature does not get its own container type. It creates a block, sets its
  type to `UILayout` and locks it. The block is the render anchor: the drawing is inserted at its
  placement, and `locked` keeps the generated body out of manual edits. The primitive is
  `structure.Service.AddLockedBlock`.