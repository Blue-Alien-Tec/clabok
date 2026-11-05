# S08 — Body Authoring, Prose Discipline and Temp

Status: implemented (2026-11-05). Delivery slices: S08a (body commands, prose discipline, block
parallelism, delivered) and S08b (temp area, flush, find, delivered).
Phase: 4. Depends on: S03 (model), S04 (store), S05 (open state, rollback), S06 (locks).
Blocks: S09, S13, S14.
PLAN refs: §3 statements 3, 5, 6; §3.3 (temp area), §3.7 (body writes), §2.5 (defect to avoid),
§3.14 (terms and grammar gate call site).

## 1. Goal

Deliver prose authoring against the block pool: write, read, replace and find text in blocks with the
block-ID rule, enforce that bodies contain no structural headings (`#`, `##`, `###`) while `####` and
deeper is text, and keep transient authoring inputs in the project `.temp` area.

## 2. In scope

| # | Requirement | PLAN |
|---|---|---|
| 1 | Bodies belong to blocks only; there is no structural body and no chapter or subchapter text | §3 (3) |
| 2 | Body commands: read (block, line range), `set`, `append`, `prepend`, `replace` (single and batch), `list` | §5, §3.13 (`internal/body`) |
| 3 | Block-ID rule: a body-writing command always takes a block ID; `0` appends a new block to the target chapter or subchapter; a valid ID overwrites that block's body; an invalid ID is rejected; the target is required for `0` and an existing block is never moved | §3.7 |
| 4 | Prose discipline: a body must not contain `#`, `##` or `###`; `####` and deeper are text, not a body unit and not a structural level | §3 (5, 6) |
| 5 | The heading rule is checkable at write time and as part of read-only acceptance; code fences are not prose headings (decide the exact rule) | §3 (6) |
| 6 | Prose headings never become structural: ToC generation and lint treat them as text (the rule is defined here, the implementation lands in S09) | §3 (6) |
| 7 | Temp area: `<tempRoot>/<bookId>/` with `tempRoot` from S02 (default `.temp/` at the project root); transient authoring inputs only; durable data stays in the store | §3.3 |
| 8 | `flush` lists stale temp inputs; deletion happens only after explicit confirmation, `--all` is the explicit project-wide sweep; it never touches the store | §3.3, §3.13 |
| 9 | `find` over bodies and manifests: fields (body, title, note, prose), context lines, count versus occurrences, case sensitivity, whole-word, files-only | §3.13 (`internal/search`) |
| 10 | UTF-8 validation, BOM refusal, and end-of-line normalization to the repository convention (`end_of_line = lf`) before a body is stored | §3.10 (determinism), `.editorconfig` |
| 11 | Every write: grammar gate hook (S13), rollback package (S05), block lock (S06), open-book requirement (S05), dry-run | §3.14, §6 |
| 12 | `body list` emits a JSON shape that is serializable for nested books: depth is a number and no domain object leaks into the response | §2.5 |

## 3. Out of scope

- Structure commands (add/move/remove of nodes) and settings: S07.
- Rendering bodies, output modes and heading emission: S09.
- Numbering and anchors: S10.
- Grammar engine, LanguageTool client and terms flow: S13 (this scope owns the call site and the
  policy switch, not the checker).
- Notes, status and other editorial regions: S12.
- Temp cleanup policy beyond `flush`; no size limit or scheduled cleanup in `PLAN.md`.

## 4. Deliverables

- `internal/body`: read, write, replace, block-ID resolution, heading rule, encoding normalization.
- `internal/temp`: temp root resolution, per-book folder, `flush` listing and confirmed deletion.
- `internal/search`: `find` over bodies and manifest fields.
- `internal/cli` command group `body` and the `find` command.
- Tests: block-ID matrix, heading matrix, batch replace atomicity, find filters, flush dry-run and
  confirmation, encoding rejection, nested `body list` JSON golden.
- This scope's implementation plan under `Planning/plans/`.

## 5. Acceptance criteria

1. `0` appends a new block to the named chapter or subchapter; the new block is returned with its ID
   and appears in the node's ordered list.
2. A valid block ID overwrites exactly that body; the block's placement and the order of siblings are
   unchanged.
3. An invalid block ID, a missing target for `0`, and a target that is not a chapter or subchapter
   each fail with a distinct stable code.
4. A body containing `#`, `##` or `###` is refused with a stable code; a body containing `####` or
   deeper is accepted and stored verbatim.
5. A batch `replace` with an expected occurrence count either applies all pairs or none; the rollback
   package restores the previous bodies.
6. `body list` on a nested book emits valid JSON with numeric depth; a golden test covers the shape
   (the §2.5 defect class).
7. `flush` without confirmation deletes nothing; with confirmation it removes only files under the
   temp root of the current store/config scope.
8. `find` reports counts and occurrences separately; `--context`, `--word`, `--case-sensitive` and
   `--files` behave as documented.
9. A CRLF body is stored with LF endings; a BOM is rejected with a stable code.
10. Every body write creates a rollback package and requires an open book; a closed book rejects the
    write.

## 6. Design decisions for this scope's planning session

- Forbidden headings: hard error, or error with an explicit override; and where the check runs
  (write, `check`, render, or all three).
- Heading detection inside fenced code blocks and inside HTML comments: how much Markdown parsing is
  acceptable without a full parser dependency.
- Whether a body may start with a `####` heading (the legacy `ahadv` pattern). `PLAN.md` classifies
  it as text; decide whether it is allowed in new prose or only documented as a legacy exception.
- `body replace` semantics: literal versus regex, replacement order and conflict handling when two
  pairs match the same region, and the meaning of `--expect N`.
- Empty bodies: allowed, refused, or normalized away.
- Body size limits (if any), and whether a huge body is split by the author or by a command.
- Temp layout: per book only, or also per command run; what `flush` considers stale (age, session,
  or "not referenced by an open book").
- `find` over manifest fields requires no open book? Decide whether it works on a closed book by
  reading the container (it does not write, and S05 forbids body reads on a closed book; reads of
  bodies must respect that rule).

## 7. Risks and constraints

- The heading rule is the boundary between prose and structure. If it is implemented as a rewrite
  (the reference implementation shifts headings), the model breaks. It must be a validation, never a
  transformation.
- `body list --json` is the reference defect named in `PLAN.md` §2.5. Test the nested case explicitly.
- Temp inputs must never become durable data by accident. `flush` deletes; the store does not.
- Do not introduce a Markdown parser dependency for the heading rule without an owner decision.
