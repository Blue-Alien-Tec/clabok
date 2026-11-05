# S03 — Implementation Plan: Domain Model and Identifiers

Status: planned (2026-11-05). Scope: [../03-domain-model-and-identifiers.md](../03-domain-model-and-identifiers.md).
Normative source: [../../PLAN.md](../../PLAN.md). Reference: [../reference-docengine.md](../reference-docengine.md).

## 1. Decisions

| # | Question | Decision | Rationale |
|---|---|---|---|
| 1 | Model split | `Manifest` (structure, regions, settings) plus `Book{Manifest, Bodies}`; bodies never live in the manifest | One JSON document for structure, one member per body; no duplication |
| 2 | Title level | The book root is a `Node` of kind `title` holding the book's top blocks and the chapters as children | Uniform placement, uniform move rules, exactly three structural levels |
| 3 | Node kinds | `title`, `chapter`, `subchapter`, `link`; kind is bound to depth (1/2/3/2) | Removes DocEngine's inferred `effective_kind` and open-ended depth |
| 4 | Link | A leaf node of kind `link`, no blocks, no children, carrying a `Ref` (`book`, `node`, `block`) | One ordered list for index entries; no parallel link array |
| 5 | Block pool | Placement lists are the block registry: a block exists when it is placed; `Book.Bodies` holds the text | No orphan blocks and no separate pool to keep in sync |
| 6 | ID type | `uint64`, encoded in JSON as a decimal string, opaque to callers | No 2^53 precision loss in JS tooling; one namespace across nodes, blocks and regions |
| 7 | ID `0` | Reserved as the creation sentinel; never valid in storage; validated on nodes, blocks and regions | `PLAN.md` §3.7 and §6 |
| 8 | ID allocation | `Allocator` interface; `model.NewSequenceAllocator(start)` for tests and `model.NewRandomAllocator()` (crypto/rand, 63-bit, nonzero) as the cooperative default | Random IDs need no coordination; the library may later supply a sequential allocator without a model change |
| 9 | Placement exclusivity | A block ID appears at most once across all nodes and regions | "Every block belongs to exactly one node or region collection" |
| 10 | Duplicate IDs | One namespace shared by nodes, blocks and regions; duplicates are findings | Cross-document references target one kind of ID |
| 11 | Manifest version | `version: 1`; a newer version is refused with the supported version in the message (DocEngine's forward-compat message pattern) | The tool owns the file; a silent downgrade is worse than a refusal |
| 12 | Decoding | Strict: unknown keys, duplicate JSON keys, trailing data rejected; BOM tolerated | Tolerant input, strict structure |
| 13 | Validation | Aggregate all issues as `model.Issue{Code, Path, Message}`; never raise for a structural defect | A shape defect is a report, not a crash (DocEngine's mixed model is the anti-pattern) |
| 14 | Settings | `numbering.enabled`, `toc.enabled`, `indexDepth` (default 2), `output` (`monolith` default, `chapter`, `part`) | `PLAN.md` §3.7; semantics stay in S09/S10 |
| 15 | Normalization | `NormalizeTitle` strips `1.`, `1.1`, `1.1.1`, `IV.` prefixes; a bare number (`2026 – …`) and `3D …` stay unchanged | `PLAN.md` §3.7; the owner's own acceptance case |
| 16 | Regions | `footer` (target book, at most one) and `notes` (target node or block); order is the render order | `PLAN.md` §3.14; more kinds arrive with S12 |
| 17 | Settings defaults on decode | `Normalize()` fills `indexDepth` and `output` when absent/zero; `toc` and `numbering` are explicit booleans | Two defaults that cannot be expressed as "unset" booleans |
| 18 | Purity | `internal/model` imports `internal/jsonstrict` only; no IO, no CLI, no rendering | Layering test in S01 |

## 2. Manifest shape

```json
{
  "version": 1,
  "id": "clabok",
  "root": {
    "id": "1",
    "kind": "title",
    "title": "CLABOK",
    "blocks": ["2"],
    "children": [
      { "id": "3", "kind": "chapter", "title": "Checks", "blocks": ["4"] },
      { "id": "5", "kind": "link", "title": "AsteroidBeltInn", "ref": { "book": "abi" } }
    ]
  },
  "regions": [
    { "id": "9", "kind": "footer", "target": { "kind": "book" }, "blocks": ["10"] }
  ],
  "settings": {
    "numbering": { "enabled": false },
    "toc": { "enabled": true },
    "indexDepth": 2,
    "output": "monolith"
  }
}
```

Bodies are not part of this document; the store keeps `blocks/<id>.md` (S04) and the runtime
`Book.Bodies` map carries the text.

## 3. Packages and files

```
internal/model/id.go          ID, ParseID, JSON string encoding
internal/model/alloc.go       Allocator, sequence and random implementations
internal/model/kind.go        Kind, parent/depth rules, OutputMode
internal/model/node.go        Node, Ref, Clone, Walk
internal/model/region.go      Region, RegionKind, Target
internal/model/settings.go    Settings, defaults, normalize
internal/model/manifest.go    Manifest, NewManifest, Decode, Encode, Clone, lookup helpers
internal/model/book.go        Book, Block, Bodies, NewBook, Clone
internal/model/validate.go    Issue, ValidationError, Manifest.Validate, Book.Validate
internal/model/title.go       NormalizeTitle
```

## 4. Tasks

1. `ID` with string JSON, `ParseID`, zero handling; tests.
2. `Allocator`: sequence and random, 10k uniqueness, never zero.
3. Kinds, depth rules, output modes.
4. `Node`, `Ref`, `Walk` with explicit `(node, depth, parent)`; no tuple that can be mistaken for a
   depth (DocEngine's defect class).
5. `Settings`, `NewManifest`, `Normalize`, `Encode`.
6. `Decode` via `jsonstrict`, version check, error codes.
7. `Book` and `Bodies`.
8. `Manifest.Validate` and `Book.Validate` with aggregated issues.
9. `NormalizeTitle` with the table tests.
10. Round-trip and property tests; `gofmt`, `go vet`, `go build`, `go test`, `go test -race`.

## 5. Validation codes

`model.manifest-version`, `model.invalid-json`, `model.unknown-key`, `model.duplicate-key`,
`model.invalid-id`, `model.missing-root`, `model.root-kind`, `model.kind-depth`,
`model.depth-exceeded`, `model.empty-title`, `model.part-on-node`, `model.link-not-leaf`,
`model.link-missing-ref`, `model.unknown-region-kind`, `model.invalid-target`,
`model.footer-count`, `model.duplicate-id`, `model.duplicate-placement`, `model.zero-id`,
`model.missing-body`, `model.orphan-body`, `model.invalid-setting`, `model.invalid-book-id`.

## 6. Acceptance mapping

| Scope criterion | Evidence |
|---|---|
| 1 four levels rejected | validate test |
| 2 block child, stored zero ID, dangling target, duplicate ID | validate table |
| 3 duplicate order position / block twice | placement test |
| 4 lossless round-trip including empty books | round-trip test |
| 5 ID allocation never zero, no collision, rekey-safe | alloc + id tests |
| 6 title normalization table | title test |
| 7 part only on chapter/link | validate test |
| 8 model imports no CLI/store/IO | layering test (S01) |

## 7. Deferred

- Index prose (per-entry prose) and further region kinds: S12 extends the manifest with a version
  bump.
- Numbering formats and anchor derivation: S10; the model stores only `numbering.enabled`.
- ID allocation from the library registry: S11 supplies an `Allocator`.
- Container encoding of the manifest: S04.

## 8. Implementation notes (2026-11-05)

- `Node.Walk` exposes the depth explicitly; there is no `(node, parent)` tuple that can be mistaken
  for a depth.
- `Manifest.Validate` aggregates; `Book.Validate` adds the body checks only after a clean structure
  validation.
- `NewManifest` normalizes the title, so a new book never carries a numeric prefix.
- Region kinds are `footer` and `notes`; the footer must target the book and is limited to one.
- `Manifest.Walk` also descends into an invalid link's children; validation rejects such a tree
  before any consumer sees it.
- `TOC.Enabled` and `Numbering.Enabled` default to false; the library applies its policy at book
  creation (S11). Only `indexDepth` and `output` are normalized.


## 9. Refinement (2026-11-05)

- The manifest version is 2. Version 2 adds `blockMeta`: an optional type token and a locked flag
  per placed block. Placement still defines existence; metadata is an overlay.
- `model.meta-orphan` rejects metadata for an unplaced block; `model.invalid-type` rejects a type
  that is not a single token of at most 64 characters.
- `locked` is a command rule, not a model rule: the model stores the flag, the structure and body
  commands refuse to change a locked block's type or body.
- `Manifest.Meta`, `SetMeta`, `IsLocked` and `BlockType` are the accessors; `Clone` copies the map.