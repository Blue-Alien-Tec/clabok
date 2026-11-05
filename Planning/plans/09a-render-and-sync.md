# S09a — Implementation Plan: Render Pipeline, Modes and Sync

Status: implemented (2026-11-05). Parent scope: [../09-rendering-and-output-modes.md](../09-rendering-and-output-modes.md).
Normative source: [../../PLAN.md](../../PLAN.md) §3.4, §3 (7), §3.7 (heading mapping, mode switch,
navigation), §6.

First delivery slice of S09. S09b adds the ASCII UI catalog and `asciiui.types` overrides.

## 1. Decisions

| # | Question | Decision | Rationale |
|---|---|---|---|
| 1 | Render purity | `Render` builds every file in memory and never writes; `Sync` compares and writes | Byte-identical re-render is testable without a disk |
| 2 | Output target | `--out <dir>` or the registry path resolved against the project directory | S11 will register real paths; `--out` keeps the slice usable now |
| 3 | Modes | monolith (default), chapter-wise, part-wise; the README always carries the index | `PLAN.md` §3.4 |
| 4 | Part grouping | chapters are grouped by their part value; a part file contains all its chapters; a chapter without a part is an error | A part is a group, not a unique per-chapter label |
| 5 | Heading mapping | monolith: `#`/`##`/`###`; part file: `#` part, `##` chapter, `###` subchapter; chapter file: `#`/`##` | `PLAN.md` §3.7 |
| 6 | Prose headings | a `####` line renders verbatim and never enters the index or the ToC | `PLAN.md` §3 (6) |
| 7 | Index | structure order, `**part**` group lines, links per mode, `index.depth` | `PLAN.md` §3.14 |
| 8 | Blocks | one blank line between blocks, empty bodies skipped | `PLAN.md` §3.7 |
| 9 | Regions | footer renders at the end of the README, notes under their node or block | S12 owns authoring; S09 renders what the model holds |
| 10 | Marker | every generated file starts with a fixed marker | Foreign-file protection and pruning |
| 11 | Sync | compares rendered bytes with disk, writes changed files, prunes marker files, refuses a hand-written file unless `--force` | `PLAN.md` §3.7, §3.14 |
| 12 | Drift state | input hash = extracted member set, output hash = rendered file set, stored in `state.json` | S04b primitives; `sync --check` and `book status` share them |
| 13 | Check | read-only: manifest validation, prose heading scan, open-book finding (non-blocking); blocking findings exit 1 | `PLAN.md` §3.5, §6 |
| 14 | Anchors | a local `Slug` until S10 owns anchors and references | S10 replaces it with the stable rule |
| 15 | Prose sharing | the prose check moved to `internal/prose`, used by body and render | Render must not import the body command package |

## 2. Error codes

`render.missing-part`, `render.not-registered`, `render.foreign-file`, `render.read-failed`,
`render.write-failed`, `render.check-failed`.

## 3. Files

```
internal/render/render.go     render modes, index, marker, sync, check
internal/render/command.go    sync, check, verify
internal/prose/prose.go       shared prose rules
internal/lifecycle/lifecycle.go  CollectMembers exported
internal/cli/layering_test.go    pure list updated
cmd/pecia/main.go             register render
```

## 4. Acceptance

| S09 criterion | Evidence |
|---|---|
| 1 README always with index | mode tests |
| 2 monolith default | render test |
| 3 chapter-wise files | file set and navigation test |
| 4 part-wise grouping and missing part | part tests |
| 7 heading mapping | mode tests |
| 8 prose heading is text | verbatim test and index exclusion |
| 10 one blank line between blocks | golden assertions |
| 12 mode switch and prune | prune test |
| 13 drift and foreign protection | sync tests |
| 14 deterministic render | byte comparison of the written file |
| 15 check acceptance | blocking-finding test |

## 5. Out of scope

- ASCII UI catalog and overrides: S09b.
- Stable anchors and references: S10.
- Library-wide sync, README as the library book: S11.
- Status, badges and layout block: S12.

## 6. Implementation notes (2026-11-05)

- The prose check was extracted into `internal/prose`; `body.CheckProse` is now a thin wrapper so
  existing tests keep their shape and render does not depend on the CLI.
- `lifecycle.CollectMembers` is exported for the input hash; the extracted member set is the
  authoritative input.
- Part files group chapters; "unique part values" means distinct groups, not one chapter per part.
- The README index and the per-file navigation are generated; no hand-written navigation is read.
