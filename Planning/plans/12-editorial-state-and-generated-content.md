# S12 Implementation Plan — Editorial State and Generated Content

Status: planned (2026-11-05)
Scope: [../12-editorial-state-and-generated-content.md](../12-editorial-state-and-generated-content.md)
Normative source: [../../PLAN.md](../../PLAN.md) §3.14, §3.7, §3.11, §3.2.

## 0. Principles

- Generated content is tool-owned; authors never hand-edit generated files or the README.
- A render is deterministic from the manifest plus the declared repository inputs.
- Notes and the footer are regions, not separate sources. Their collection order is the render
  order.
- Every mutation is open-only, snapshots, validates, supports `--dry-run` and reports through the
  standard envelope.

## 1. Delivery slices

| Slice | Content | Why |
|---|---|---|
| **S12a** | Manifest v3, index prose, `region` commands (footer, notes), rendering of prose | The editorial core; uses the existing region renderer |
| **S12b** | `asciiui add/set/remove` authoring on locked `UILayout` blocks | Closes the existing render feature's authoring gap |
| **S12c** | Status vocabulary, assignments, legend and structural rendering | Opt-in editorial state |
| **S12d** | Badge detectors, overrides and the generated badge region | Repository facts |
| **S12e** | Repository-layout block from `git ls-files`, per-path prose | Last; may be a follow-up if it does not fit |

S12e is the explicitly deferred part of the scope. If it slips, S12 still ships the editorial core.

Delivered: S12a (2026-11-05, corrected 2026-11-06), S12b and S12c (2026-11-06) and S12d (2026-11-06,
the last slice of this planning segment).

Delivery decision (2026-11-06): S12d closes this segment, then M6 is closed with the `1.2.0-dev`
bump. The deferred M5 `1.1.0-dev` bump stays deferred by the owner decision. S12e (repository-layout
block) moves to the next planning segment as its first task; the plan already allows S12 to ship
without it.

## 2. Decisions

### D1 — Manifest v3

`model.Version` becomes 3. `Decode` accepts 2 and 3 and normalizes to 3; `Encode` writes 3. Version 2
manifests read unchanged (no data migration beyond the version field). The new fields are optional,
so S12b–S12e add them without another version bump:

- `Node.ProseBlock string` — index-prose **block reference**; the prose lives in a block (`PLAN.md` §3
  statement 5). Resolved (2026-11-06, correction): the delivered S12a field `Node.Prose` stored the
  text on the node and contradicted `PLAN.md` §3 statement 5. It is replaced by `proseBlock`; the
  text now lives in a block that a `prose` region collection with the node as its target owns, so the
  block has a placement, a lock scope and a rollback path like every other body.
- `Node.Status string` and `BlockMeta.Status string` — S12c.
- `Manifest.Status StatusConfig` — S12c.
- `Manifest.Badges []Badge` — S12d.
- `Manifest.Layout *LayoutConfig` — S12e.

### D2 — Regions

- **Footer**: at most one per book, target `book`, exactly one block. `region set --kind footer`
  replaces the block; `region clear --kind footer` removes the region and its body.
- **Notes**: one region per target (node or block), ordered blocks. `region notes --target
  node|block <id>` appends a note block; `region remove --target node|block <id>` removes the whole
  notes region. Multiple notes are multiple blocks in one region.
- **Prose**: at most one region per node, target `node`, exactly one block. `Node.ProseBlock`
  references that block; clearing the prose removes the block and the region.
- Validation adds: prose format, one notes region per target, footer block count.
- The renderer already renders footer and notes; S12a only adds authoring and prose.

### D3 — Mutation machinery

Extend `structure` with `SetProse`, `SetFooter`, `ClearFooter`, `AddNote`, `RemoveNote`. They run
through a new `Step` op set on the existing `run` path, so locking, snapshot, validation and dry-run
stay in one place. `mutation` gains a `bodies` map for created and updated region blocks, so the text
is written in the same atomic step (not an empty file first).

### D4 — Index prose

The block referenced by `Node.ProseBlock` renders in the `Description` column of the index table.
The index shape is a derived state, not a stored setting: without index prose and without status the
book index renders the plain list (part lines, nested indentation); as soon as one entry carries
prose (S12c adds status to the same condition) it renders the table `Document | Description`. Each
`part` value starts a new table group with a bold part line. Hierarchy is a `&emsp;` indent in the
`Document` cell, so it survives without numbering. The per-file `**Contents**` ToC stays a flat list
in every case and never carries prose. Entry text is plain: inline code, emphasis and links in a
title are stripped for the index, the ToC and reference links, so a link never contains a second
link; headings keep their inline formatting. The entry order and depth are unchanged.

Resolved (2026-11-06): the prose stays a block body; only the index presentation is derived from
the data. S12a first delivered the table unconditionally; the list fallback is the derived state.

### D5 — Commands

Two-word commands, actions as positionals where needed:

```
pecia prose set   --book <id> --node <id> --prose <t> | --file <path|->
pecia prose clear --book <id> --node <id>
pecia region set  --book <id> --kind footer --body <t> | --file <path|->
pecia region clear --book <id> --kind footer
pecia region notes   --book <id> --target node|block --id <id> --body <t> | --file <path|->
pecia region remove  --book <id> --target node|block --id <id>
```

All are mutating, open-only and dry-run aware. `prose` and `region` are registered from
`internal/editorial`; the primitives live in `internal/structure`.

### D6 — Status (S12c)

The vocabulary is tool-owned and fixed: `final`, `review`, `wip`, `draft`, in that order, each with a
label, a shields.io badge color and a one-line description. `Manifest.Status.Enabled` is the per-book
switch; a disabled book renders nothing and ignores assignments, so switching it off is lossless.
`Node.Status` and `BlockMeta.Status` hold a state id.

Commands (all open-only and dry-run aware):

```
pecia status enable|disable --book <id>
pecia status set   --book <id> --target node|block --id <id> --state <id>
pecia status clear --book <id> --target node|block --id <id>
pecia status show  --book <id>
```

Rendering: the badge follows a heading, fills the `Status` cell of the index table and prefixes a
block body; the legend (`**Status**` plus a `Status | Meaning` table of badge and description)
precedes the index in every output mode. An index column exists only when it has a source: `Document | Status`
without index prose, `Document | Status | Description` with prose. The per-file ToC stays a list and
carries no badge. An unknown assignment is `model.unknown-status` and blocks `check`.

### D7 — Badges (S12d)

Badges are declared in the manifest (`id`, `label`, optional explicit `message`, optional `link`).
`id` selects a fixed detector; a non-empty `message` overrides it. The detectors are `go` (the
`go` directive in `go.mod`), `module` (the `module` path), `license` (`LICENSE`/`COPYING` → SPDX
label), `remote` (the origin url in `.git/config` → `owner/repo`) and `version`
(`internal/buildinfo`). A missing source omits the badge; `check` reports `badge-unresolved`
(non-blocking) and `sync` renders the rest.

The badge line is generated only in the library README, between the top navigation and the H1. Each
badge is a shields.io image (`![label: message](url)`), wrapped in the `link` when one is set; the
whole README is generated, so a hand edit never survives a render. The resolved values participate
in the input hash, so a change in `go.mod`, `LICENSE` or `.git/config` dirties the README.

Commands (open-only, dry-run aware):

```
pecia badge add    --book <id> --id <detector> [--label <l>] [--message <m>] [--link <url>]
pecia badge remove --book <id> --id <detector>
pecia badge show   --book <id>
```

### D8 — Layout (S12e)

Opt-in per book, generated from `git ls-files` (tracked paths only), with depth/include/exclude and
per-path prose. No git repository is an informative no-op. The block is generated, not stored as
authoring prose. This is the last slice and may move to a follow-up.

### D9 — ASCII UI authoring (S12b)

The spec is not a body: it is a container member at `layouts/asciiui/<blockID>.asciiui`. The authoring
command creates a locked `UILayout` block and writes the spec in one `structure` mutation, so a render
anchor never exists without its drawing. The drawing is rendered inside a fenced `text` code block
between the `asciiui` markers, so Markdown preserves its spacing instead of collapsing it into prose.
`SetLayout` updates the spec only; `RemoveLayout` removes the
block and the spec. The lifecycle treats the specs like bodies: close packs them, open extracts them,
and an undo restore replaces the extracted layout tree. The `asciiui` package stays pure (parser,
renderer, `Validate`); the command surface is registered from `editorial`. The built-in defaults name
only generic widgets; `asciiui.types.<kind>` names the project type and always wins over an explicit
generic label, so `Button` becomes `UIButton` project-wide, while an explicit non-generic label stays
a one-node override. `widget=` names a UI widget type, never a content or domain object.

## 3. S12a tasks

1. Manifest v3, `Node.ProseBlock`, the `prose` region, prose validation and the v2 upgrade.
   (Delivered 2026-11-06; replaces the earlier `Node.Prose` text field.)
2. `structure` step ops `prose`, `footer`, `footer-clear`, `note`, `note-remove`; `mutation.bodies`;
   write created and updated bodies with their content.
3. `internal/editorial`: `prose` and `region` commands, text reading from `--text`/`--file`/stdin.
4. Render the book index as a list when it has neither prose nor status, and as a
   `Document | Description` table once prose exists; the ToC stays a list.
5. Tests: v3 round-trip and v2 upgrade; prose validation and the region reference; footer
   set/replace/clear; notes add/remove; dry-run writes nothing; open refusal; index table golden,
   prose column and ToC exclusion.
6. Docs: S12 scope status, handover contracts, planning book chapter.

## 4. S12b tasks

1. `structure` gains `AddLayoutBlock`, `SetLayout` and `RemoveLayout` plus the step ops
   `add-layout`, `layout`, `layout-remove`; a layout spec and its block are written in one atomic
   step, and the undo snapshot covers the spec.
2. `asciiui add|set|remove` is registered from `internal/editorial` (the `asciiui` package stays pure
   and adds only `Validate`): `add --parent --file`, `set --block --file`,
   `remove --block`; all open-only, dry-run aware and validated before any write.
3. Lifecycle packs `layouts/asciiui/<blockID>.asciiui` into the container on close, extracts it on
   open and removes the extracted tree on close and on an undo restore.
4. Tests: spec and block atomicity, locked `UILayout` metadata, set/replace, remove (spec and block),
   dry-run writes nothing, non-layout target rejected, a closed book renders the drawing and reopen
   restores the spec.
5. Docs: S12 scope status, handover contracts.

## 5. S12c tasks

1. `model/status.go` holds the fixed vocabulary and `Manifest.Status`; `Node.Status` and
   `BlockMeta.Status` are additive manifest v3 fields.
2. `structure` gains `SetStatus` and `SetStatusEnabled` (step ops `status`, `status-config`) and a
   read-only `StatusSummary`; validation rejects unknown states while the system is on and ignores
   assignments while it is off.
3. `status enable|disable|set|clear|show` is registered from `internal/editorial`.
4. Render: badge on headings, a `Status` column in the index (the table becomes the index shape when
   status is on), a badge before a block body, and the legend above the index in every mode.
5. Tests: model validation (known/unknown/disabled), structure lifecycle (enable, set, clear, dry-run,
   lossless disable), render golden (column, heading, block marker, legend, ToC stays a list), CLI
   end-to-end.

## 6. S12d tasks

1. `model.Badge` and `Manifest.Badges`; validation for the id token and duplicate ids.
2. `internal/badge`: the five detectors, explicit-message precedence, shields.io URL escaping and
   the unresolved reasons.
3. `structure.SetBadge`, `RemoveBadge` and the read-only `Badges` view, all through the atomic run.
4. The `badge add|remove|show` command registered from `editorial`; the badge line in the library
   README above the H1; resolved values in the input hash.
5. `check` reports `badge-unresolved` as a non-blocking finding.
6. Tests: detector fixtures, model validation, structure lifecycle, render (library only, above the
   title, unresolved omitted) and the CLI end-to-end.

## 7. Acceptance mapping

| Scope criterion | Slice |
|---|---|
| 1 footer single slot, replace/clear | S12a |
| 2 notes adjacency and ordering | S12a |
| 3 note removal | S12a |
| 4 index-prose block reference and index table rendering | S12a |
| 5 status opt-in and structural rendering | S12c |
| 6 badge override | S12d |
| 7 layout deterministic, no git no-op | S12e |
| 8 no header/preamble | already true; test grep |
| 9 rollback package and open-book requirement | S12a–S12e |

S12b (ASCII UI authoring on locked `UILayout` blocks) has no separate scope criterion; it closes the
authoring gap of the existing ASCII UI render feature.

## 8. Verification

`gofmt -l .`, `go vet ./...`, `go build ./...`, `go test ./... -count=1`; then `pecia sync --check`
and `check` on both books. The version bump and CI remain deferred by the owner decision.
