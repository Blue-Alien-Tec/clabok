# S10b — Implementation Plan: Rekey

Status: implemented (2026-11-05). Parent scope: [../10-numbering-anchors-references.md](../10-numbering-anchors-references.md).
Normative source: [../../PLAN.md](../../PLAN.md) §3.14 (rekey), §3.7 (IDs), §6.

Second and last delivery slice of S10.

## 1. Decisions

| # | Question | Decision | Rationale |
|---|---|---|---|
| 1 | Command | `rekey --book <id> <old> <new>` | One explicit operation; no other command changes an ID |
| 2 | Atomicity | book-exclusive lock, one undo package, validation, then file effects and the manifest write | Same contract as every other mutation |
| 3 | Rewrite scope | placements, region targets and region block lists, link refs, block metadata, the block body file name, and `[[id:old]]` in every body of the book | A rekey must leave no reference behind |
| 4 | New ID | must be nonzero, unused in the book, and different from the old one; `old == new` is a no-op | Duplicate IDs stay impossible |
| 5 | Kind | works for nodes and blocks | `PLAN.md` §3.14 |
| 6 | Dry run | computes and validates, writes nothing | Uniform receipt contract |
| 7 | Cross-book rewrite | deferred to S11, which owns the registry and multi-book operations | The rewrites need the library context |
| 8 | Failure order | bodies first, manifest last; the undo package covers the pre-rekey state | A crash leaves a detectable state, never a silently broken book |

## 2. Error codes

`structure.invalid-id`, `structure.id-in-use`, `structure.unknown-target`,
`structure.read-failed`, `structure.write-failed`.

## 3. Acceptance

| S10 criterion | Evidence |
|---|---|
| 10 rekey changes the ID and rewrites references | block and node tests |
| 10 the body file follows the block ID | rename test |
| 10 metadata follows the block ID | block rekey test |
| 10 a failed rekey leaves the book untouched | dry-run and conflict tests |
| 8 IDs stay unique | conflict test |

## 4. Out of scope

- Library-wide rewrite across registered books: S11.
- Bulk rekey: not part of the target model.

## 5. Implementation notes (2026-11-05)

- The body map is rewritten in memory and written back once; the old body file is removed after the
  new one is written.
- Region targets are rewritten in the same pass as link references, so a notes collection stays
  attached to a renamed block.
- The command rides the same CLI surface (`payloadOf`, dry-run receipt) as the other structure
  commands.
