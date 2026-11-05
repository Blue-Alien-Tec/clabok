# S07 — Structure and Book Settings Commands

Status: implemented (2026-11-05). Delivery slices: S07a (add, rename, move, remove, book settings,
delivered) and S07b (declarative `apply` plans, delivered).
Phase: 4. Depends on: S03 (model), S04 (store), S05 (open state), S06 (locks). Blocks: S08, S09, S11,
S14.
PLAN refs: §3.7 (uniform commands, placement, move arguments, reparenting, settings live with the
book, no pin), §3 (4, 9, 10), §3.14 (no pin), §6 (rollback for every write).

## 1. Goal

Deliver the complete structure-authoring surface: uniform `add`, `rename`, `move`, `remove` over
nodes and blocks, explicit placement, reparenting across levels, and the book-settings commands that
replace direct manifest editing — all through locks, dry-run and rollback.

## 2. In scope

| # | Requirement | PLAN |
|---|---|---|
| 1 | `add`, `rename`, `move`, `remove` operate on structure nodes and blocks; the node type follows the target parent (book -> chapter, chapter -> subchapter, subchapter -> block); no per-type commands | §3.7 |
| 2 | Placement is list order: a node or region holds an ordered block-ID list; blocks carry no placement data; a move edits two lists and never touches a body | §3.7 |
| 3 | Chapters and subchapters have an order among their siblings; reordering and reparenting of chapter and subchapter nodes stay possible | §3 (4), §3.7 |
| 4 | Move arguments: `--child-of <node>` appends, `--index N` places explicitly, `--before/--after <block-id>` places relatively | §3.7 |
| 5 | Reparenting keeps the node ID and its block list; its children move with it; a move that would create a fourth structural level is rejected | §3.7 |
| 6 | Moving a block assigns its placement and keeps its body byte-identical and its ID unchanged | §3 (4) |
| 7 | Book settings commands for `numbering`, `toc`, `index.depth` and `output`: stored in the book manifest, changed only through commands; the library holds defaults and policy, never per-book values | §3.7 |
| 8 | `pin` is removed; the generated index follows the structure order | §3.7 |
| 9 | Declarative batch plans (`apply`): a validated, atomic plan covering structure and content, with one rollback point, delegating to the same services as the individual commands | §3.13 (`internal/apply`) |
| 10 | Every mutating command: `--dry-run` receipt, rollback package (S05), lock per S06, requires an open book (S05), rejects stale state | §3.12, §6 |
| 11 | Stable error codes for: unknown target, wrong parent type, level limit, duplicate title within a parent, invalid index, dangling block ID, stale state, closed book | §3.7 |
| 12 | JSON responses include the resulting order (affected lists) so tooling does not have to re-read the book | §3.10 |

## 3. Out of scope

- Body text and body writes: S08.
- Rendering, heading emission, index generation, mode switches: S09.
- Numbering semantics, anchor derivation, reference rewriting: S10.
- Library-level defaults and their application across books: S11.
- ID allocation internals: S03 (model) plus the store/registry owner; this scope calls the allocator.
- Applying a plan's prose bodies: the content part of `apply` calls S08's body service.

## 4. Deliverables

- `internal/structure`: add, rename, move, remove, reorder, reparent, invariants, dry-run receipts.
- `internal/cli` command group for the four commands and for the settings commands under `book`.
- `internal/apply`: plan schema, validation, atomic execution, one rollback point.
- Tests: a matrix per command and node type; move argument combinations; level-limit rejections;
  body-identity assertion after a block move; dry-run leaves the book byte-identical; rollback after
  a failed plan.
- This scope's implementation plan under `Planning/plans/`.

## 5. Acceptance criteria

1. `add` creates the type dictated by the parent: a chapter under the book, a subchapter under a
   chapter, a block under a subchapter; an attempt to add a chapter under a subchapter is refused.
2. Moving a chapter with children moves the whole subtree; IDs and block lists are unchanged.
3. A move that would create a fourth structural level fails with a stable code and changes nothing.
4. `--child-of`, `--index`, `--before` and `--after` each place the unit exactly; a combination that
   is contradictory is rejected before writing.
5. Moving a block between two nodes leaves both bodies byte-identical (hash comparison).
6. `--dry-run` produces a receipt and leaves the container and the extracted bodies byte-identical.
7. A failing `apply` plan leaves no partial change: one rollback point, the book is as before.
8. `book set numbering|toc|index.depth|output` round-trips through the manifest; an invalid enum value
   is refused; the command works only on an open book.
9. `pin` is absent from the command surface and from the manifest schema.
10. A closed book rejects every structure command with `book.not-open`.
11. Two processes adding to the same chapter serialize; the final order is one of the two valid
    orders, never a lost entry.

## 6. Design decisions for this scope's planning session

- `remove` on a node with children: cascade, reject, or require an explicit `--recursive`. `PLAN.md`
  does not decide it; pick one and state the rationale.
- Whether `add` can create a node together with its first blocks, or creates the node only.
- Exact command grammar: positional arguments versus flags for the target and the title, and how a
  block target is distinguished from a node target.
- How `--before/--after` behave when the reference block is in another node (reject, or move into the
  referenced block's node).
- Whether a title must be unique within its parent (the model may allow duplicates; the renderer
  anchors must stay unique, which S10 solves; decide whether uniqueness is a hard rule).
- `apply` plan schema and whether it is versioned like the config.
- Whether settings commands accept the value as a positional or a flag, and how `index.depth` is
  bounded.
- Receipt shape for a move: the two affected lists, or the whole affected subtree.

## 7. Risks and constraints

- The uniform command surface is a product statement. Do not add `add-chapter`, `move-block` or other
  per-type aliases.
- Structure changes are the highest-risk mutations. Every one needs a rollback package and a dry-run
  receipt, including the successful path.
- Do not let `apply` become a second implementation of structure logic; it must call the same
  services.
