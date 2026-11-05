# S10 — Numbering, Anchors and References

Status: implemented (2026-11-05). Delivery slices: S10a (numbering, anchors, references, delivered).
Revision (2026-11-05, owner decision): IDs are immutable; the S10b rekey slice was removed. Placement
is a collection edit and duplicate ownership is a `sync --check` finding. The rekey text below is
historical. Cross-book reference resolution and link entries, deferred to S11, were delivered
retrospectively on 2026-11-05. The numbering hierarchy is fixed (chapters 1., subchapters 1.1).
Phase: 5. Depends on: S09 (render pipeline, heading emission). Blocks: S11, S12.
PLAN refs: §3 (8), §3.7 (numbering, anchors, references, reorder), §6 (references by stable ID,
anchors derived).

## 1. Goal

Make numbering an opt-in render-time property of the structure, keep anchors and references
independent of numbers and of titles, and provide ID-based cross-document references. IDs are
immutable: placement changes never rewrite a reference.

## 2. In scope

| # | Requirement | PLAN |
|---|---|---|
| 1 | Numbering is opt-in; the library holds defaults and policy, the effective values are stored in the book manifest (storage by S07, policy by S11) | §3 (8), §3.7 |
| 2 | Numbers are added at render time when numbering is enabled; titles are stored without numbers | §3.7 |
| 3 | A numeric prefix in an incoming title (`1.`, `1.1`, `IV.`) is stripped during normalization, not rejected (normalization rule owned by S03) | §3.7 |
| 4 | Numbering applies to the structure tree; a `part` value creates no heading and does not affect numbering | §3 (10) |
| 5 | Anchors are number-independent: the heading anchor derives from the title or the ID, never from the number | §3.7 |
| 6 | Enabling, disabling or changing numbering never breaks a reference | §3.7 |
| 7 | Cross-document references target stable IDs of structure nodes and blocks; the visible anchor is derived at render time | §3.7, §6 |
| 8 | IDs are library-managed and unique across all books; global uniqueness is a deliberate constraint | §3.7 |
| 9 | Reordering regenerates numbering and all derived anchors and references in the same render | §3.7 |
| 10 | IDs are immutable: a move/reparent changes the ordered collections and leaves the ID and every reference valid; there is no rekey | §3.14 |
| 11 | Reference resolution: resolve an ID to its current location, detect dangling references, and report them as findings | §3.7, §6 |
| 12 | Deterministic anchor generation including Unicode titles, collision handling within a document and across documents | §3.7 |
| 13 | `link` entries participate in references; the link target is an ID, not a path | §3 (10) |

## 3. Out of scope

- Numbering and output-mode settings storage and command grammar: S07.
- Library defaults storage and policy: S11.
- Heading emission mechanics and output files: S09.
- ID creation, global uniqueness enforcement and the ID type: S03.
- Editorial regions and status: S12.
- Legacy Markdown links in the untouched legacy books: explicitly out of scope.

## 4. Deliverables

- `internal/refs`: ID reference type, resolution, rewriting, dangling detection, anchor derivation,
  collision handling.
- Numbering formatter integrated into the render pipeline (per-level format, optional per-book
  style), with the "titles are stored without numbers" guarantee.
- Reference resolution and rewriting helpers; no ID-changing operation.
- Tests: numbering toggle golden (headings change, anchors and references do not), reorder golden,
  duplicate-ownership finding, dangling reference finding, Unicode anchor fixtures, collision
  fixtures.
- This scope's implementation plan under `Planning/plans/`.

## 5. Acceptance criteria

1. With numbering off and on, the set of anchors and the resolved references is identical; only the
   visible numbers in headings differ (golden comparison).
2. Changing a numbering format (for example `1.` versus `1.1`) changes headings and leaves every
   anchor and reference valid.
3. Reordering two chapters renumbers them and updates all derived anchors and references in the same
   render; no reference dangles afterwards.
4. A move or reparent keeps the ID and every reference valid; duplicate ownership across books is a
   finding, not a rename.
5. A reference to a removed ID is reported as a finding with the referencing location.
6. Two chapters with the same title produce distinct anchors, deterministically and stably across
   renders.
7. A Unicode title produces a deterministic anchor and the same anchor on every platform.
8. Stored titles contain no numeric prefix after normalization; a title entered as `1.2 Ziel`
   renders as `1.2 Ziel` when numbering is enabled and as `Ziel` when it is disabled.
9. `part` values never appear in numbering and never emit a heading.

## 6. Design decisions for this scope's planning session

- Per-level number format and whether it is configurable per book or fixed by the library policy.
- Anchor derivation: title slug, stable ID, or title slug with an ID suffix on collision. The choice
  determines link stability across renames; `PLAN.md` requires number independence and stable-ID
  references, not a specific anchor text.
- Slug algorithm: case folding, whitespace, punctuation and Unicode transliteration. Must be
  deterministic across platforms.
- Collision scope: per generated file, per book, or per library.
- Reference syntax in prose and in manifest links, and whether it may target a block or only a node.
- Whether dangling references are an error or a non-blocking finding, and in which command they
  surface (`check`, `verify`, render).
- Whether numbers can be part of generated file names (coordination with S09).

## 7. Risks and constraints

- Numbers creeping into anchors is the defect `PLAN.md` names. Test the toggle as a first-class
  acceptance criterion, not as an afterthought.
- IDs are immutable. Do not expose an ID-changing flag anywhere; a duplicate is reported by
  `sync --check`, never repaired by renaming.
- Legacy books are not read by the new tool. A reference to a legacy anchor is not a case the new
  system has to support.
