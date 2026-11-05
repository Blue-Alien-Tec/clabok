# DocEngine as Reference

Status: reference notes (2026-11-05). DocEngine is read-only reference material; it is never
modified. Source: `F:\UnityProjects\AsteroidBeltInn\Tools\agents\docenginelib` (2.4) and
`Tools/agents/tests`.

This document records what CLABOK adopts, what it deliberately does differently, and why. A scope
plan cites the relevant rows instead of re-deriving them.

## 1. What DocEngine does right

| Area | Reference | What CLABOK keeps |
|---|---|---|
| Container determinism | `storage.py` | ZIP with a fixed member order, a fixed timestamp `(1980,1,1,0,0,0)`, per-member SHA-256, byte-identical re-save |
| Concurrent writes | `storage.py:_merge` | Save applies only this process' member deltas to the on-disk state; a changed member the process did not touch is a stale conflict, not a silent overwrite |
| Locking | `locks.py` | OS advisory lock (`msvcrt.locking` / `fcntl.flock`) in a dot-folder next to the store; the OS releases it when the process dies, so there is no stale-lock cleanup and no deletion race; holder metadata names the writer in the busy message |
| Atomic writes | `textsafe.atomic_write`, `storage.py:_write_atomic` | Temp sibling, fsync, replace; failure leaves the previous file intact |
| Text safety | `textsafe` | BOM tolerance on input, explicit encoding errors, LF output, retry on Windows replace contention |
| Error surface | `cli_common.py:_error_code`, `_error_hint` | A stable machine code plus a short actionable hint on every error |
| Dry run | `cli_common.py:_diff_and_save` | Serialize before and after, print the diff, write only when asked |
| Unknown-key discipline | `model.py:_require_keys` | Manifest and config reject unknown keys; forward-compat message names the supported version |
| Numbering at render time | `numbering.py` | Numbers exist only in the rendered result; the manifest stores titles without numbers |
| Rollback packages | `rollback.py` | Snapshot source plus owned outputs, cap the pool, restore deletes the package it used |
| Terms confirmation | `terms.py`, `commands_terms.py` | `set`/`delete` are requests; a separate confirmed commit writes |
| Grammar server sharing | `grammar_server.py` | One shared local server, state/lock/log in the OS temp directory, degraded check is a warning |
| Stdin handling | `cli_common.py:read_stdin_text` | BOM accepted, non-UTF-8 rejected with an actionable message |
| Test layout | `tests/` | One module per subsystem, fixtures in-process, no network |

## 2. Where DocEngine is structurally weak

| Weakness | Evidence | Consequence in DocEngine |
|---|---|---|
| One type for node and body | `model.py:105-146` (`Block` carries title, kind, file, link, content, body membership) | Structure and prose are the same unit; moving text rewrites bodies |
| Kind is inferred, not bound to depth | `model.py:135-146` (`effective_kind`) | `chapter` at depth 3, `section` at depth 2; `MAX_DEPTH = 6` leaves the hierarchy open |
| File names on any node | `model.py:889` | `file` may appear below depth 1, where the renderer ignores it |
| Heading shifting | `render.py` (`shift_headings`) | A `####` in prose is rewritten as structure |
| Number inside the anchor | `numbering.py:44-46` (`anchor_for(book, number, title)`) | Toggling numbering breaks anchors and therefore references |
| Mixed error model | `model.py:811-923` (`validate` raises for shape, returns findings for the rest) | A structural defect crashes the command instead of being reported |
| Parent/depth confusion | `model.py:273-281` (`iter_blocks` yields `(block, parent)`), `commands_body.py:62` | `body list --json` crashes on nested books |
| Storage-mode setting | `storage.py` (`archive` / `files`) | A model concern leaks into a setting users must understand |
| Header/preamble as string sources | `model.py:301-308` | Region content sits outside the block pool |
| Text-first CLI | `cli_common.py`, `cli_parser.py` | `--json` is opt-in; tooling is the second-class citizen |
| One exit code for everything | `cli.py:156-158`, `174-191` | A gate reject and a broken store look identical to automation |
| Code derived from message text | `cli_common.py:199-269` | Renaming a message changes the machine code |

## 3. Consequences for CLABOK

| Decision | Scope |
|---|---|
| Structure is data; blocks are the only bodies; three levels bound to kind | S03 |
| Placement as ordered block-ID lists; a move edits lists, never bodies | S03, S07 |
| Opaque library IDs, JSON as strings; no slug-derived identity | S03 |
| Aggregate validation findings (`model.*` codes) instead of raising | S03 |
| JSON is the canonical output; `--text` is the human form | S01 |
| Exit codes distinguish success, failure, usage and reject | S01 |
| Stable error codes from a namespace, hint as data, never derived from the message | S01 |
| Container and merge/stale behaviour as in `storage.py`, but without a storage-mode setting | S04, S05 |
| OS advisory locks as in `locks.py`; the daemon backend must offer the same semantics | S06 |
| Numbers only at render time; anchors derived from the title or ID, never from the number | S10 |
| Regions as blocks in ordered collections; no header/preamble | S12 |
| Snapshot/rollback semantics as in `rollback.py`, with the two-tier cap from `PLAN.md` | S05 |

## 4. Explicit non-adoptions

- `pin`: removed (`PLAN.md` §3.7).
- `header`/`preamble` region sources: removed (`PLAN.md` §3.14).
- Storage modes `archive`/`files`: replaced by open/close (`PLAN.md` §3.7).
- Derived slug IDs: replaced by library-managed opaque IDs.
- `shift_headings`: removed; prose headings are text.
- The C# ASCII-UI parity test: not carried over (`PLAN.md` §2.4).
- Import, adopt and migrate: not part of the first delivery (`PLAN.md` §3.1).
