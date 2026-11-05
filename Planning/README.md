# CLABOK Scope Map

Status: scope breakdown (2026-11-05).
Normative source: [../PLAN.md](../PLAN.md).

This folder decomposes the conceptual target model in `PLAN.md` into independently plannable
scopes. `PLAN.md` stays the single normative record of the target model and of the owner decisions;
these documents only cut that model into planning units and record what each unit must deliver.

## 1. Why this folder exists

`PLAN.md` is one document of roughly 650 lines covering fourteen decision blocks, eight gap rows and
an unrestricted target model. Planning or implementing it in one pass would mix foundation,
storage, authoring, rendering and delivery concerns in a single plan. Each scope document below is
the brief for exactly one planning session: it fixes the boundary, the referenced PLAN statements,
the deliverables and the acceptance criteria, and it lists the implementation decisions that the
session has to resolve.

## 2. Ground rules

1. `PLAN.md` is normative. A scope document refines it into planning units and never changes a
  product statement. A contradiction is a defect in the scope document, not a decision.
2. Scope documents resolve **implementation design only** (command grammar, JSON shapes, formats,
  algorithms, test strategy). Product behaviour is already decided in `PLAN.md` §3.
3. A scope owns its boundary. Work that belongs to another scope is listed under "Out of scope"
  with the owning scope ID, never implemented locally.
4. One scope, one plan. The output of planning a scope is an implementation plan (resolved design
  decisions, task breakdown, test plan) written to `Planning/plans/<scope-id>-<slug>.md`.
5. Status flows `draft` -> `planned` -> `implemented`. The status line of each scope document and
  the status table in this file are updated together.
6. Adding, splitting or merging a scope is allowed when planning shows the cut is wrong. The scope
  catalogue, the coverage table (§5) and the dependency order (§4) are updated in the same change.

## 3. Scope catalogue

| ID | Scope | Phase | Depends on | Status | PLAN refs |
|---|---|---|---|---|---|
| [S01](01-foundation-and-cli-contract.md) | Foundation and CLI contract | 1 | — | implemented | §3.8, §3.9, §3.10, §3.13 |
| [S02](02-configuration.md) | Configuration | 1 | S01 | implemented | §3.11 |
| [S03](03-domain-model-and-identifiers.md) | Domain model and identifiers | 2 | S01 | implemented | §3, §3.7 (identity, placement, normalization), §3.14 (rekey) |
| [S04](04-central-store-and-containers.md) | Central store and containers | 3 | S02, S03 | draft | §3.2, §3.7 (store layout), §6 |
| [S05](05-open-close-and-rollback.md) | Open/close lifecycle and rollback tiers | 3 | S04, S06 | draft | §3.5, §3.7 (open state, rollback) |
| [S06](06-concurrency-and-coordination.md) | Concurrency and coordination | 3 | S04 | draft | §3.12, §6 |
| [S07](07-structure-and-book-settings.md) | Structure and book settings commands | 4 | S03, S04, S05, S06 | draft | §3.7 (uniform commands, placement, move, settings), §3.14 (no pin) |
| [S08](08-body-authoring-and-prose.md) | Body authoring, prose discipline and temp | 4 | S03, S04, S05, S06 | draft | §3 (3, 5, 6), §3.3, §3.7 (body writes), §2.5 |
| [S09](09-rendering-and-output-modes.md) | Rendering and output modes | 5 | S04, S07, S08 | draft | §3.4, §3.7 (heading mapping, mode switch, navigation), §3 (7), §3.14 (index prose) |
| [S10](10-numbering-anchors-references.md) | Numbering, anchors and references | 5 | S09 | draft | §3.7 (numbering, anchors, references, reorder), §3.14 (rekey) |
| [S11](11-library-docs-and-cutover.md) | Library book, documentation location and cut-over | 6 | S02, S04, S05, S09, S10 | draft | §3.1, §3.6, §3.8, §3.9 |
| [S12](12-editorial-state-and-generated-content.md) | Editorial state and generated content | 7 | S09 | draft | §3.14, §3 (4) |
| [S13](13-grammar-and-terms.md) | Grammar gate and term list | 7 | S02, S08 | draft | §3.14 (terms and grammar), §3.11 (grammar endpoint) |
| [S14](14-tui.md) | Terminal UI | 8 | S07, S08, S09 | draft | §3.10 (TUI) |

## 4. Delivery order and milestones

The phases are the recommended implementation order. Scopes inside a phase are independent of each
other unless the table states otherwise.

```
Phase 1   S01 --> S02
Phase 2   S03
Phase 3   S04 --> S06 --> S05
Phase 4   S07 --> S08
Phase 5   S09 --> S10
Phase 6   S11
Phase 7   S12, S13
Phase 8   S14
```

| Milestone | Scopes | Exit criterion | Version after |
|---|---|---|---|
| M1 Skeleton | S01, S02, S03 | A configurable binary with a stable output contract and a validated model, but no book storage | `0.1.0-dev` (delivered) |
| M2 Durable books | S04, S05, S06 | A book survives open/close, two processes edit different blocks concurrently | `0.2.0-dev` |
| M3 Authoring | S07, S08 | A book can be structured and written end to end | `0.3.0-dev` |
| M4 Output | S09, S10 | Every output mode renders deterministically, references stay valid across numbering changes | `1.0.0-dev` |
| M5 Dogfooding | S11 | The CLABOK `README.md` is generated by `pecia` and `sync --check` is clean | `1.1.0-dev` |
| M6 Editorial | S12 | Footer, notes, status and generated content are available | `1.2.0-dev` |
| M7 Grammar | S13 | The grammar gate and term confirmation flow are available | `1.3.0-dev` |
| M8 TUI | S14 | The terminal UI is available on the same core | `1.4.0-dev` |

M5 is the cut-over milestone and the first proof that the model works on a real book. The extension
milestones do not block it.

M1 is delivered (2026-11-05): `cli`, `config`, `jsonstrict` and `model` build and pass their tests;
`gofmt`, `go vet`, `go build` and `go test` are green. The current version is `0.1.0-dev`.

### 4.1 Version policy

- **One place.** The version lives only in `internal/buildinfo/buildinfo.go` (`var Version`). No
  `go.mod` change, no second constant, no version string in `cmd/`.
- **Bump only at a milestone close.** The bump rides the commit that closes the milestone, after
  the milestone's scopes are green; that push to `main` is the CI run worth having. No bump per
  scope, per commit, per fix or per document change.
- **Reaching `1.4.0-dev`.** The ladder above ends at `1.4.0-dev` when the last scope (S14) closes.
  `1.0.0-dev` at M4 marks the complete authoring and rendering core; M5 proves it on a real book;
  the remaining minors add optional capability.
- **No release bumps.** No git tags, no `-ldflags` release stamping, no GitHub release. The value
  stays a `-dev` string; the tag-based release path remains a later decision.
- **Bump semantics.** `0.x` means the model, storage or authoring surface may still change; `1.0`
  marks the complete core; each later minor adds capability without a breaking change.
- **Independent versions.** The JSON schema (`cli.SchemaVersion`), the configuration version and
  the manifest version are contract versions and never move with the product version.
- **If a milestone splits.** The bump lands when the milestone's exit criterion is met, not when
  the last file is merged. A milestone that is dropped merges its version into the next one; the
  ladder still ends at `1.4.0-dev`.

## 5. Coverage of `PLAN.md` §3

| PLAN | Statement | Scope |
|---|---|---|
| §3.1 | Hard cut, no migration, library book is the exception | S11 |
| §3.2 | Central working-data store | S04 |
| §3.3 | Temp area `.temp/` | S08 |
| §3.4 | Output modes (monolith, chapter-wise, part-wise) | S09 |
| §3.5 | Open/close lifecycle and rollback tiers | S05 |
| §3.6 | Documentation location (`Docs/`) | S11, S02 (docs root) |
| §3.7 | Engineering defaults | S03, S04, S05, S07, S08, S09, S10, S11 |
| §3.8 | Naming (CLABOK, `pecia`) | S01 |
| §3.9 | Project boundary (DocEngine unchanged) | S01 |
| §3.10 | Go, CLI contract, TUI | S01, S14 |
| §3.11 | Configuration | S02 |
| §3.12 | Concurrency and locking | S06 |
| §3.13 | Code organization | S01 (baseline), every scope (its packages) |
| §3.14 | Generated content and editorial state | S12, S13, S10 (rekey) |

`PLAN.md` §2 (verified current state) is reference material, not a scope. §2.5 is an explicit
constraint on S08: the JSON defect of the reference implementation must not be reproduced.

## 6. Invariant ownership (`PLAN.md` §6)

| Invariant | Owner | Enforced in |
|---|---|---|
| Deterministic containers, byte-identical re-save | S04 | container tests |
| Open/close safety, detectable interruption, no lost write | S05 | crash-simulation tests |
| Identical semantics for both coordination backends | S06 | shared conformance suite |
| Concurrent edits of different blocks, stable lock order | S06 | multi-process tests, `-race` |
| Acyclic package dependencies, domain never imports the CLI | S01 | dependency review, optional `depguard` |
| Machine-readable default output, schema-versioned, stable | S01 | golden JSON tests |
| Block ID `0` reserved for creation | S03 | model validators, S08 command tests |
| Drift detection between input and output | S04 | hash tests, S09 sync tests |
| Rollback package for every write | S05 | command tests in S07, S08, S12 |
| References by stable ID, anchors derived at render time | S10 | numbering-toggle test |
| Grammar gate on prose-writing commands | S13 | gate matrix tests |
| `check` / `verify` are read-only acceptance | S09 | acceptance tests |
| Term list confirmation flow | S13 | pending/commit tests |
| CLABOK test suite | S01 | every scope delivers tests |
| Config validated before any write, paths relative to config | S02 | config tests |

## 7. Cross-scope contracts

These interfaces are the seams between scopes. The owning scope publishes them; every consumer
depends on the interface, never on the implementation.

| Contract | Owner | Shape |
|---|---|---|
| Model | S03 | Book, structural node, block, region collection, ID, validators. Pure data, no IO, no rendering |
| Store | S04 | Container read/write, atomic file writes, member hashes, book hash pairs. No book semantics |
| Coordinator | S06 | Acquire a lock for one resource (book, node, block) with timeout; reader snapshot |
| Open state | S05 | Is a book open, where its extracted bodies live, close barriers |
| Render | S09 | Pure function: model + bodies + settings -> generated files and their hashes |
| References | S10 | Resolve and rewrite stable IDs; anchor derivation |
| Grammar gate | S13 | Check prose, return findings; enforce the configured policy |
| Configuration | S02 | Effective values (paths, limits, modes, overrides), resolved and validated before use |

A scope that needs more than its contract from a neighbouring scope documents the need and the
owning scope adds it. Silent duplication of another scope's responsibility is a defect.

## 8. Definition of done (applies to every scope)

- `gofmt -l .` empty, `go vet ./...`, `go build ./...`, `go test ./...`, `go test -race ./...` green.
- Tests cover the scope's normative statements, including the error paths and the dry-run path.
- Every mutating command supports `--dry-run` and leaves the book byte-identical when it runs.
- JSON output is the default; `--text` is the human form; diagnostics go to stderr; exit codes follow
  the S01 table.
- A `doc.go` for every package created, with the responsibility stated in one paragraph.
- The scope's own documentation is updated (tool docs are authored with CLABOK itself from S11 on;
  before that, `Planning/plans/` and package `doc.go` are the record).
- The version is unchanged unless the milestone closes; then it is bumped in
  `internal/buildinfo/buildinfo.go` only (see §4.1).
- No new dependency without an explicit owner decision (single static binary, `CGO_ENABLED=0`).
- No change to DocEngine, to the legacy books or to the Unity project.

## 9. Global exclusions

These are excluded for every scope and are not repeated per document:

- DocEngine is not modified; `F:\UnityProjects\AsteroidBeltInn\Tools\agents` is read-only reference.
- The legacy books under `Assets/Docs` are not read, changed, moved or deleted.
- Import, `library import`, `book adopt` and any migration of existing books.
- Detailed command syntax is a per-scope design decision, but it may not contradict `PLAN.md` §3.
- Custom agents, SRR and the Unity code base.
- Header and preamble regions; cleanup of legacy navigation, index or banner residue; the
  storage-mode command (replaced by open/close).

## 10. Change control

- A scope document never amends `PLAN.md`. If planning reveals a product-level gap, stop and take
  the question to the owner; the answer is recorded in `PLAN.md` first, then in the affected scopes.
- Design decisions resolved in a planning session are recorded in that scope's implementation plan
  under `Planning/plans/`, not silently in code.
- Scope status changes are recorded here and in the scope header in the same commit.
