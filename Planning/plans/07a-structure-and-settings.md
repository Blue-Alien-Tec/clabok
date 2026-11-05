# S07a — Implementation Plan: Structure Commands and Book Settings

Status: implemented (2026-11-05). Parent scope: [../07-structure-and-book-settings.md](../07-structure-and-book-settings.md).
Normative source: [../../PLAN.md](../../PLAN.md) §3.7 (uniform commands, placement, move),
§3 (4, 9, 10), §3.14 (no pin).

First delivery slice of S07. S07b adds declarative `apply` plans.

## 1. Decisions

| # | Question | Decision | Rationale |
|---|---|---|---|
| 1 | Command grammar | `add --book <id> --title <t> [--parent <node>] [--index N | --before <id> | --after <id>]`, `rename --book <id> <target> --title <t>`, `move --book <id> <target> --child-of <node> [--index N] | --before/--after <id>`, `remove --book <id> <target> [--recursive]`, `book set <book-id> <key> <value>` | One uniform surface; the book is context, the unit is the target |
| 2 | Type from the parent | the parent kind decides: title -> chapter, chapter -> subchapter, subchapter -> block; a link or a block cannot hold units | `PLAN.md` §3.7 |
| 3 | Book root | `--child-of root` / `--parent root` names the title node; `Append(0)` in the service | The root ID is an implementation detail the author should not need |
| 4 | Placement | list order only; `--before/--after` resolve the anchor's parent and position; `--index` validates against the current list length | `PLAN.md` §3.7 |
| 5 | Reparenting | a node keeps its ID, its blocks and its children; its kind is retagged to the new depth; a fourth level is rejected | Kind is bound to depth |
| 6 | Cycles | moving a node into its own subtree is rejected before any kind check | The most meaningful error wins |
| 7 | Block moves | only the two placement lists change; the body file is never touched | Structure is data |
| 8 | Remove | a non-empty node requires `--recursive`; removing a node removes its block bodies | No accidental subtree loss |
| 9 | Write order | add: manifest, then the empty body; remove: bodies, then the manifest | A crash leaves an empty body rather than a dangling placement |
| 10 | Locks | every structure and settings command takes the book exclusively; undo snapshot inside the lock | Structural changes serialize; `Snapshot` takes no lock to avoid self-deadlock |
| 11 | Undo | every real mutation snapshots first (`struct.*`, `book.setting`) | `PLAN.md` §3.5 |
| 12 | Validation | the mutated manifest is validated before anything is written; a failure is `book.invalid` (reject) | A command must never persist an invalid book |
| 13 | Settings | `numbering on|off`, `toc on|off`, `index-depth >= 1`, `output monolith|chapter|part` | `PLAN.md` §3.7 |
| 14 | Receipts | JSON reports kind, target, parent, index and the resulting order; `--dry-run` writes nothing | Tooling needs the resulting order; dry run is the contract |
| 15 | IDs | existing IDs are collected into a set; the allocator retries on collision | Global uniqueness stays the invariant |
| 16 | Block metadata | `block type --book <id> <block> <type>` sets or clears the type; `add --type` sets it on a new block; a locked block rejects both. Moves and removals stay allowed, removal drops the metadata, and types may repeat across blocks | The type is a manifest overlay; `locked` protects content, not placement |

## 2. Error codes

`structure.unknown-target`, `structure.parent-invalid`, `structure.level-exceeded`,
`structure.cycle`, `structure.node-not-empty`, `structure.block-no-title`, `structure.root`,
`structure.invalid-placement`, `structure.invalid-index`, `structure.anchor-mismatch`,
`structure.empty-title`, `structure.invalid-value`, `structure.unknown-setting`,
`structure.link-parent`, `structure.read-failed`, `structure.write-failed`,
`structure.alloc-failed`, plus `book.invalid` as the reject code.

## 3. Files

```
internal/structure/structure.go   Service: add, rename, move, remove, set setting, helpers
internal/structure/command.go     CLI: add, rename, move, remove, book set, payloads
internal/structure/*_test.go      service and command tests
internal/lifecycle/lifecycle.go   exported ManifestMember, LockBookExclusive/Shared
internal/lifecycle/command.go     FromEnv exported
internal/cli/layering_test.go     pure-package list adjusted
cmd/pecia/main.go                 register the structure commands
```

## 4. Acceptance

| S07 criterion | Evidence |
|---|---|
| 1 uniform commands, type from the parent | add tests for chapter, subchapter and block |
| 2 list order, sibling order | move/ordering tests |
| 3 reorder and reparent | move tests, retag test |
| 4 move arguments | before/after/child-of/index tests |
| 5 reparent keeps ID and children, fourth level rejected | level test, move test |
| 6 block move keeps the body | body comparison |
| 7 settings round-trip, invalid values rejected | settings test |
| 8 no pin | absent from the surface and the model |
| 9 dry-run and undo | dry-run byte comparison, undo package test |
| 10 open book required | `RequireOpen` error path |
| 11 resulting order in JSON | receipt payload |

## 5. Out of scope

- Declarative `apply` plans: S07b.
- Body writes and prose rules: S08.
- Library-level defaults: S11.
- Region authoring: S12.

## 6. Implementation notes (2026-11-05)

- `model` has no block kind: a block is a placed ID, not a node. The service uses the string
  `"block"` for its unit kind and converts to `model.Kind` only for real nodes.
- Tests found four real defects before they reached the CLI: an empty title blocked block creation;
  a block used as a parent reported `unknown-target` instead of `parent-invalid`; `Append(0)` was
  indistinguishable from an empty placement (the index now distinguishes them); a cycle was
  reported as a parent-kind error because the destination was resolved first.
- The layering test now names the pure packages only. `config`, `lifecycle`, `structure` and
  `coordinator` carry their command surface and therefore import the CLI by design.
- A block body is created only after the manifest write; a crash between the two leaves a placed
  block with an empty body, which close handles.
