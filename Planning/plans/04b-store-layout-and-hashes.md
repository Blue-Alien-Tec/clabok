# S04b — Implementation Plan: Store Layout and Hash Pairs

Status: implemented (2026-11-05). Parent scope: [../04-central-store-and-containers.md](../04-central-store-and-containers.md).
Normative source: [../../PLAN.md](../../PLAN.md) §3.2, §3.7 (store layout), §6 (drift detection).

Second delivery slice of S04. S04c adds the registry primitives and `diff`/`export`.

## 1. Decisions

| # | Question | Decision | Rationale |
|---|---|---|---|
| 1 | Layout type | `store.Layout{Root}` resolved once from the effective config; `Layout.Book(id)` returns all per-book paths | One place for the layout, no path building in command code |
| 2 | Root | must be absolute; `NewLayout` rejects an empty or relative root | The config already resolves absolute paths; a relative root would silently anchor to the working directory |
| 3 | Book paths | `<root>/<id>/`, container `<id>/<id>.book`, `blocks/`, `layouts/`, `state.json`, `rollback/<id>/` | Matches `PLAN.md` §3.7; the container name is stable and readable |
| 4 | Registry and terms | `<root>/library.json`, `<root>/terms.txt`, `<root>/rollback/` | Names are layout constants; S11 and S13 own their content |
| 5 | Invalid book ID | `Layout.Book` returns a `store.invalid-book-id` error instead of building a path | A path function must not silently accept `..` or separators |
| 6 | Hash function | SHA-256 over, per member in sorted order: name, `0x00`, 8-byte little-endian length, data | Order-independent, unambiguous at name/data boundaries, cheap for book sizes |
| 7 | Input hash | the caller hashes the authoritative member set: container bytes when closed, extracted bodies when open | S04b provides the primitive; S05 decides which set is authoritative |
| 8 | Output hash | the same function over the rendered file set, keyed by relative path | One definition for both sides of the drift check |
| 9 | State storage | `<bookId>/state.json` with `inputHash` and `outputHash`, atomic write | Per book, next to the container, no registry dependency |
| 10 | Empty state | writing a state with both hashes empty deletes the file | "No state" is the absence of a file, not a sentinel value |
| 11 | Compare | `clean`, `dirty-input`, `dirty-output`, `dirty-both`, `no-state` | What `sync --check` and `book status` need; no rendering required |
| 12 | Atomic writer | one `atomicWrite` used by the container and the state file | One code path for temp, fsync, chmod and rename |
| 13 | Absolute member names | a member name containing `:` is invalid | Rejects `C:/...` drive paths inside the container |

## 2. Error codes

`store.invalid-root`, `store.invalid-book-id`, `store.state-corrupt`, `store.read-failed`,
`store.write-failed`.

## 3. Tasks

1. `Layout` and `BookPaths` with the path constants.
2. Root validation (absolute, cleaned).
3. Book ID validation through `model.BookID.Valid`.
4. `HashMembers` and `HashBytes`.
5. `State`, `Status`, `ReadState`, `WriteState`, `Compare`.
6. Extract `atomicWrite` and use it in `Container.Save` and `WriteState`.
7. Reject `:` in member names; extend the parse and write tests.
8. `gofmt`, `go vet`, `go build`, `go test`.

## 4. Acceptance

| S04 criterion | Evidence |
|---|---|
| 2 fixed layout below the root | layout test asserts every path |
| 3 identity decoupled from the output path | `Layout.Book` takes the ID only; no output path anywhere in the store |
| 6 input/output hash pair per book | state round-trip and compare matrix |
| 10 atomic writes | shared `atomicWrite`; no temp files left behind |
| 11 no absolute paths in the container | member-name test with `C:/...` |

## 5. Out of scope

- Registry file content and library policy: S11 (primitives in S04c).
- Which member set is authoritative while a book is open: S05.
- Rendering and `sync --check`: S09.
- Terms content: S13.

## 6. Implementation notes (2026-11-05)

- `NewLayout` requires an absolute root. The effective configuration always resolves to absolute
  paths, so a relative root here would silently anchor to the working directory.
- Registry and terms names are layout constants (`library.json`, `terms.txt`). S11 and S13 may
  refine the content, but S05 already reads the paths.
- `HashMembers` prefixes every member with its name, a NUL, an 8-byte length and the data. The
  boundary test (`ab`/`c` versus `a`/`bc`) pins the property that matters.
- `State.Compare` treats an empty stored output hash as a mismatch, so a book that was never
  rendered reports `dirty-output` rather than `clean`.
- `atomicWrite` is shared by the container and the state file; the temp prefix is `.store-` and no
  temp file survives a successful write.
- Member names containing `:` are invalid, which rejects `C:/...` drive paths inside a container in
  addition to the traversal rules from S04a.

