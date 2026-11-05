# S03 — Domain Model and Identifiers

Status: implemented (2026-11-05). Plan: [plans/03-domain-model-and-identifiers.md](plans/03-domain-model-and-identifiers.md).
Phase: 2. Depends on: S01. Blocks: S04, S07, S08, S09, S10, S11.
PLAN refs: §3 statements 2, 3, 4, 5, 6, 10; §3.7 (placement storage, identity, sibling order,
normalization, uniform commands); §3.14 (regions are blocks, rekey); §6 (block ID `0`, references by
stable ID).

## 1. Goal

Define the complete in-memory and persisted model — book, structural nodes without bodies, a flat
block pool, region collections, ordered placement and globally unique IDs — with validators that
make every structural invariant machine-checkable before any storage, command or render code exists.

## 2. In scope

| # | Requirement | PLAN |
|---|---|---|
| 1 | Book = manifest metadata (title, settings, structure, region collections) plus a block pool | §3.2 |
| 2 | Structural nodes: title (book title level), chapter, subchapter. They are values (title, kind, position) and have no body and no body field | §3 (2) |
| 3 | A maximum of three structural levels; a move or insert that would create a fourth is rejected | §3.7 |
| 4 | The block pool is flat: `{id, body}` only. Blocks carry no placement data and cannot have children | §3 (3), §3.7 |
| 5 | Structural nodes and region collections hold ordered lists of block IDs; list order is the render order and the sibling order; no position fields | §3 (4), §3.7 |
| 6 | Region collections: stable ID, ordered block-ID list, and a target that is a node, a block or the book | §3 (4), §3.14 |
| 7 | Vocabulary: `part` is an optional value on a chapter or link (index grouping only, never a node, never a body); `link` stays a manifest entry; inline `content` becomes a block; `file` is legacy and not modelled | §3 (10), §5 |
| 8 | ID rules: block ID `0` is a reserved creation sentinel and invalid when stored; generated IDs are nonzero; IDs are library-managed and globally unique across all books; IDs are stable by default | §3.7, §6 |
| 9 | Node and library entry IDs follow the same stability and uniqueness rules as block IDs, with their own namespaces if the design needs them | §3.7 |
| 10 | Title normalization: a numeric prefix (`1.`, `1.1`, `1.1.1`, `IV.`) is stripped during normalization, never rejected; titles are stored without numbers | §3.7 |
| 11 | Book settings are stored in the manifest (`numbering`, `toc`, `index.depth`, `output`) with their defaults; their semantics are owned by S09 and S10 | §3.7 |
| 12 | Validators as pure functions returning stable error codes: level limit, block-child rule, dangling block ID, duplicate ID, stored block ID `0`, duplicate sibling order entry, region target validity | §3.7 |
| 13 | Rekey: an operation type that changes an ID and rewrites every reference to it; the rewrite itself is owned by S10 | §3.14 |
| 14 | Manifest encoding is a data-only concern: serializable and round-trippable, no IO, no rendering, no locking | §3.13 |

## 3. Out of scope

- Container format, checksums, atomic writes: S04.
- Open/close state and extracted body files: S05.
- Command grammar (`add`, `move`, `rename`, `remove`, settings commands): S07.
- Body text handling, heading rule and write commands: S08.
- Renderer, output modes and heading emission: S09.
- Numbering semantics, anchor derivation and reference resolution: S10.
- Registry, library policy and defaults: S11.
- Title normalization of incoming legacy content: not needed, there is no import.

## 4. Deliverables

- `internal/model`: book, node, block, region collection, settings, ID types, rekey operation type.
- Validators for every invariant in the in-scope list, with stable error codes and aggregate
  reporting (all violations, not just the first).
- Manifest schema (JSON) plus a documented, versioned schema description.
- Fixture builders for tests in later scopes (a minimal book, a nested book, a book with regions and
  notes, an invalid book per invariant).
- Round-trip tests and property tests (random tree -> encode -> decode -> equal, and reject/invariants
  hold for every generated case).
- This scope's implementation plan under `Planning/plans/`.

## 5. Acceptance criteria

1. A book with four structural levels fails validation with a stable code; the same tree created via
   a command (S07) is rejected before writing.
2. A block with a child, a stored block with ID `0`, a dangling block ID, and a duplicate ID each
   fail validation with distinct stable codes.
3. A tree with two nodes claiming the same order position or the same block twice in one list fails.
4. Round-trip of every fixture is lossless, including empty books, empty regions and a book whose
   structure is only a title.
5. ID generation never yields `0`, never collides across two books that are validated together, and
   survives a rekey of another ID.
6. Title normalization strips `1.`, `1.1`, `1.1.1` and `IV.` prefixes and leaves `2026 – Year of X`
   and `3D Printing` unchanged (the latter is a test case, not an assumption).
7. `part` is representable on a chapter or link and rejected on a subchapter or block.
8. The model package imports neither `internal/cli`, nor `internal/store`, nor any IO package.

## 6. Design decisions for this scope's planning session

- ID representation: integer, string, or a typed value with a namespace prefix. Decide the one shape
  that survives global uniqueness and JSON.
- Whether node and block IDs share one counter/namespace with globally unique values, or use disjoint
  prefixes. `PLAN.md` requires global uniqueness; the encoding is open.
- Where the next-ID allocator lives: the model cannot allocate without the registry (S11) or the
  store (S04). Decide the interface and the owner.
- How `link` entries are represented (target book/node/block, title for display) and whether a link
  can be a block instead of a manifest entry.
- Manifest JSON: one document versus split documents (structure, settings, regions). Affects
  container members in S04 and lock granularity in S06.
- Whether `part` uniqueness is validated in the model (part-wise output needs unique parts) or only
  at render time in S09.
- Region collection shape: one collection type with a target discriminator, or a node-attached
  collection field. `PLAN.md` says regions are blocks in an ordered collection with a target.
- `index.depth`, `toc` and `output` enum values: define them here so S09 can consume them.
- How a rekey request is represented so S10 can execute the reference rewrite atomically.

## 7. Risks and constraints

- The model is the contract every other scope compiles against. Changes after S04 are expensive;
  spend the design budget on the ID representation, the placement lists and the manifest split.
- `PLAN.md` forbids structural bodies. Do not add a compatibility field for "chapter body"; the hard
  cut means no legacy shape has to be representable.
- Block ID `0` is a sentinel, never data. Do not encode it as "unset" in a way that can round-trip
  into storage.
- Keep the model free of file paths. A book's output path belongs to the store and the registry.
