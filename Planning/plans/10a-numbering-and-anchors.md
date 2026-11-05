# S10a — Implementation Plan: Numbering, Anchors and References

Status: implemented (2026-11-05). Parent scope: [../10-numbering-anchors-references.md](../10-numbering-anchors-references.md).
Normative source: [../../PLAN.md](../../PLAN.md) §3 (8), §3.7 (numbering, anchors, references,
reorder), §6.

First delivery slice of S10. S10b adds rekey.

## 1. Decisions

| # | Question | Decision | Rationale |
|---|---|---|---|
| 1 | Number format | fixed decimal hierarchy: chapters `1.`, subchapters `1.1`; parts are never numbered | Predictable and render-time only; no stored numbers |
| 2 | Numbering toggle | `numbering.enabled` from the book settings; numbers exist only in the rendered heading | `PLAN.md` §3.7 |
| 3 | Anchors | derived from the stored title (Unicode letters/digits, runs of others become one dash), never from the number | Enabling or changing numbering cannot break a link |
| 4 | Anchor emission | an explicit `<a id="...">` line before each structural heading | Markdown auto-anchors would include the number prefix |
| 5 | Collisions | per rendered document, suffixed `-2`, `-3` in walk order | Deterministic, no cross-file clash |
| 6 | Index links | use the same anchor map and the file map | Index and headings cannot drift apart |
| 7 | Prose references | `[[id:N]]`; expanded to `[Title](href)` | Stable-ID references, anchors resolved at render |
| 8 | Cross-file hrefs | `file#anchor` when the target lives in another rendered file | Works in chapter and part modes |
| 9 | Dangling references | stay literal in the output and are a blocking `refs.dangling` finding | No silent rewrite, no crash |
| 10 | Link entries | `Ref.Node`/`Ref.Block` are validated against the book; cross-book refs were deferred to S11 and delivered retrospectively (2026-11-05) through the library-wide layout map | `PLAN.md` §3.7 |
| 11 | Removal cleanup | removing a node or block drops notes whose target disappears and their blocks | A removed target must not leave an invalid region |
| 12 | Reorder | numbering recomputes; anchors stay title-based | `PLAN.md` §3.7 |

## 2. Error codes

`refs.dangling` (finding), plus the existing `render.*` and `structure.node-not-empty` paths.

## 3. Files

```
internal/refs/refs.go        anchor derivation, assigner, target resolution, [[id:N]] pattern
internal/render/render.go    numbering map, explicit anchors, ref expansion, dangling checks
internal/structure/structure.go  note cleanup when a target is removed
```

## 4. Acceptance

| S10 criterion | Evidence |
|---|---|
| 1 opt-in numbering, titles number-free | toggle test |
| 4 parts stay unnumbered | part render test |
| 5 anchors independent of numbers | numbered/plain comparison |
| 6 toggling numbering keeps references | same anchors and hrefs |
| 7 references by stable ID | expansion test |
| 8 cross-file references | chapter-mode test |
| 9 reorder | numbers follow, anchors stay |
| 12 collisions and Unicode | collision and anchor tests |
| 11 dangling references | check finding |
| 13 removal cleanup | structure test removing a node with notes |

## 5. Out of scope

- Rekey: S10b (removed by the owner decision of 2026-11-05).
- Library-wide reference resolution: S11, delivered retrospectively on 2026-11-05.
- Configurable per-level number styles: not in the target model; the fixed hierarchy is the decision.

## 6. Implementation notes (2026-11-05)

- The explicit anchor line is the only way to keep links stable when numbering is enabled, because
  Markdown auto-anchors include the rendered number.
- The regions cleanup was forced by the new tests: removing a node left a notes region pointing at
  it, which made the manifest invalid. Removal now drops such regions and their blocks.
- `refs.Pattern` is the single source for the prose reference syntax; render and check share it.
