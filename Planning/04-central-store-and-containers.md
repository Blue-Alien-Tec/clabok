# S04 — Central Store and Containers

Status: draft, not planned.
Phase: 3. Depends on: S02 (paths), S03 (model). Blocks: S05, S06, S07, S08, S09, S11.
PLAN refs: §3.2 (central store), §3.7 (store layout, extracted bodies, rollback location), §6
(deterministic containers, drift detection), §2.1 (current container shape), §2.5 (defect to avoid).

## 1. Goal

Make the store the single durable home of all working data — one configurable root, a deterministic
book container per book, a registry, terms, rollback storage and layout sources — with byte-identical
round-trips, per-member hashes and drift detection, and without any book semantics.

## 2. In scope

| # | Requirement | PLAN |
|---|---|---|
| 1 | One store root (default `_CLABOK`, configurable in S02); the docs path carries rendered output only, no `_source` folders | §3.2 |
| 2 | Fixed layout below the root: `<bookId>/`, `<bookId>/blocks/` (extracted bodies, above S05), `<bookId>/layouts/`, `rollback/<bookId>/`, the registry, `terms.txt` | §3.7 |
| 3 | Book identity is decoupled from the output path: the book ID names the store folder; the output path is an attribute, not the identity | §3.2 |
| 4 | Container: deterministic encoding with fixed member order, fixed timestamp and a per-member SHA-256; re-saving an unchanged book is byte-identical | §6 |
| 5 | Reader/writer API: read all members, read one member, write a member set, verify checksums, reject a tampered or truncated container with a stable error | §6 |
| 6 | Input/output hash pairs per book, stored where `sync --check` and `book status` can compare them without re-rendering | §6 |
| 7 | `diff` between two container states (member-level added/changed/removed) and `export` of container members to a directory | §5, §3.13 |
| 8 | Registry persistence primitives (file read/write, atomic replace, version field); registry semantics and policy belong to S11 | §3.2, §3.7 |
| 9 | `terms.txt` and layout sources live in the store; their content semantics belong to S12 and S13 | §3.2, §3.14 |
| 10 | Atomic file writes for every store mutation (temp sibling + rename), with the same guarantees on Windows and Unix | §6 |
| 11 | No absolute paths inside the container; no machine-specific metadata; no timestamps that break determinism | §6 |
| 12 | `.gitignore` already excludes `_CLABOK/` and `.temp/`; the store must not require additional ignore rules | §3.2, §3.3 |

## 3. Out of scope

- Open/close state machine, extraction lifecycle and rollback tier policy: S05.
- Locking and multi-process coordination: S06.
- Registry entries, library policy and defaults: S11.
- Rendering, output hashing from a render pass and `sync` behaviour: S09 (it consumes the hash API).
- Temp input handling and `flush`: S08.
- Terms content and grammar: S13. Layout block content: S12.
- Any read of legacy `Assets/Docs` containers: excluded by the hard cut.

## 4. Deliverables

- `internal/store`: container codec, member hashing, atomic writer, book hash pairs, store layout
  resolution from the S02 config.
- `internal/store`: `diff` and `export` primitives.
- Registry file codec with a version field and atomic replace (semantics delegated to S11).
- Error codes for: container corrupt, checksum mismatch, member missing, store not found, store not
  writable, absolute path in container, write conflict.
- Fixture containers (minimal, nested, large, corrupted, truncated) under `testdata/`.
- Benchmark for container write/read to catch accidental quadratic behaviour.
- This scope's implementation plan under `Planning/plans/`.

## 5. Acceptance criteria

1. Create a book container, read it back, write it unchanged: the bytes are identical (golden file
   and SHA-256 comparison).
2. Reordering the in-memory member map does not change the output; a test permutes insertion order.
3. A corrupted member, a truncated file and a wrong checksum each fail with a distinct stable code
   and never return partial data as success.
4. A container written on Windows reads identically on Linux in CI (fixture is committed).
5. `diff` reports added, changed and removed members between two states.
6. `export` writes exactly the members and paths of the container.
7. A failed write leaves the previous container intact (simulated failure between temp write and
   rename).
8. The store root resolves from the S02 config; two books in one store do not share any mutable file.
9. No package reads the store root from the working directory or from the environment.

## 6. Design decisions for this scope's planning session

- Container encoding: standard `archive/zip` with normalized headers versus a custom framed format.
  `PLAN.md` fixes determinism, not the encoding. Check that the chosen encoding can pin the member
  timestamp and the compression level.
- Member layout: one manifest member plus `blocks/<id>.md`, or a per-node manifest split. It follows
  the manifest decision from S03.
- Checksum member: a separate checksum index member, or a checksum per member entry in the archive
  comment. Decide the tamper evidence.
- Hash pair definition: what exactly is hashed for "input" and "output" (manifest bytes, body bytes,
  rendered bytes) and where the pair is stored (book folder vs registry).
- Registry file format and versioning; how a registry entry records the output path, kind, and the
  library's own policy fields.
- How `bookId` is derived (slug, assigned number, hash) and whether it is user-visible.
- Whether `export` is a first-delivery command or a later convenience (it is not required by any
  `PLAN.md` statement; keep it small).
- Compression level and its effect on determinism and size.

## 7. Risks and constraints

- The reference implementation's `body list --json` defect (§2.5) comes from storing the parent
  object where a depth number is expected. Any "list" API in the store must expose a hashable or
  serializable shape; add a golden JSON test for every list command.
- Determinism is easy to lose through map iteration and `time.Now()`. Pin both in the codec and test
  them.
- Windows rename semantics differ from Unix; the atomic writer needs its own test on both platforms.
- The store is a working area, not a backup. Rollback packages are the safety net (S05), not the
  store's job.
