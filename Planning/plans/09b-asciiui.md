# S09b — Implementation Plan: ASCII UI Catalog and Anchor Rendering

Status: implemented (2026-11-05). Parent scope: [../09-rendering-and-output-modes.md](../09-rendering-and-output-modes.md).
Normative source: [../../PLAN.md](../../PLAN.md) §3.7 (ASCII UI type names), §3.14 (ASCII UI anchor),
§3.11 (`asciiui.types`).

Second and last delivery slice of S09.

## 1. Decisions

| # | Question | Decision | Rationale |
|---|---|---|---|
| 1 | Spec format | an indented tree of 2 spaces per level; `title`, `legend`, and widget lines; `#` starts a comment | Compact, hand-authorable, testable |
| 2 | Widget kinds | `panel`, `row`, `col`, `button`, `label`, `input`, `checkbox`, `radio`, `list`, `image`, `value`, `separator` | Generic names only; no project types |
| 3 | Defaults | a built-in generic label per kind (`button` -> `Button`) | No project-specific names in the default catalog |
| 4 | Overrides | `asciiui.types` maps a generic kind to a project label; a blank entry keeps the default; per-node `widget=` wins | `PLAN.md` §3.11, §3.7 |
| 5 | Geometry | panels are bordered boxes, rows are side-by-side with two-space gaps, cols stack; every line is padded and right-trimmed deterministically | Aligned ASCII without a layout dependency |
| 6 | Legend | opt-in via `legend true`; lists label and resolved widget name | Makes the override visible |
| 7 | Anchor | a locked block with type `UILayout` renders its spec instead of its body; the drawing is wrapped in `<!-- asciiui:<id> -->` markers | `PLAN.md` §3.14; the block is the anchor, not a special region |
| 8 | Spec location | `<store>/<book>/layouts/asciiui/<blockId>.asciiui` | Durable store data, separate from the locked block body |
| 9 | Missing spec | `render.missing-layout` fails the render and is a blocking `check` finding | A declared anchor without a drawing is a book defect |
| 10 | Parity test | none against project C# types | `PLAN.md` §2.4 |

## 2. Error codes

`asciiui.invalid-spec`, `asciiui.unknown-kind`, `render.missing-layout`.

## 3. Files

```
internal/asciiui/asciiui.go       catalog, parser, layout engine
internal/asciiui/asciiui_test.go  parse, render, overrides, legend
internal/render/render.go         anchor rendering, overrides, check finding
internal/render/*_test.go         anchor integration and missing-spec tests
internal/render/command.go        Overrides from asciiui.types
```

## 4. Acceptance

| S09 criterion | Evidence |
|---|---|
| 16 catalog with generic defaults | resolve tests |
| 16 overrides from config | render test with `asciiui.types` |
| 16 anchored ASCII splicing | marker and drawing assertions |
| 16 no C# parity test | no such test and no project names in defaults |
| 6 rendering at the target position | block placed in the chapter, drawing in its place |

## 5. Out of scope

- The `asciiui add|set|remove` authoring commands: S12 owns the feature and calls
  `structure.AddLockedBlock`.
- Stable anchors and references: S10.
- Index prose: the block reference arrives with S12.

## 6. Implementation notes (2026-11-05)

- The anchor rendering replaces the body in the renderer's body map, so the existing block placement
  and index logic needs no special case.
- Several `UILayout` blocks per book are valid; each reads its own spec file keyed by block ID.
- The engine has no external dependency and no runtime codegen, which keeps the render path
  AOT-friendly for the IL2CPP-style build model.
- `check` reuses the same render function, so an invalid spec is reported with the exact code.
