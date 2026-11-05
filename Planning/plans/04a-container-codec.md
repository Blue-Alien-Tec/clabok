# S04a — Implementation Plan: Deterministic Container Codec

Status: implemented (2026-11-05). Parent scope: [../04-central-store-and-containers.md](../04-central-store-and-containers.md).
Normative source: [../../PLAN.md](../../PLAN.md) §3.2, §3.7 (store layout), §6.

This is the first of three delivery slices of S04. S04b adds the store layout, book identity and the
hash pairs; S04c adds the registry primitives and `diff`/`export`.

## 1. Decisions

| # | Question | Decision | Rationale |
|---|---|---|---|
| 1 | Compression | every member is `zip.Store` (uncompressed) | No compressor output in the bytes: identical containers across platforms and Go versions. The store is local and gitignored, so size is not a cost |
| 2 | Metadata member | `container.json` with exactly `{"format": 1}` | The book's own versions live in the manifest; a tool version inside the container would break byte-identity after a tool upgrade |
| 3 | Checksums | one `checksums.json` member, member name to lowercase hex SHA-256, sorted keys | One place to verify; a missing, orphaned or wrong entry is detectable |
| 4 | Member order | `container.json`, `checksums.json`, then the remaining names sorted bytewise | Fixed order, independent of map iteration and insertion order |
| 5 | Timestamp | `1980-01-01T00:00:00Z` for every member | The MS-DOS epoch; no wall clock in the bytes |
| 6 | Member names | forward slashes only, no leading `/`, no `\`, `path.Clean(name) == name`, not reserved | Rejects `..`, absolute names and Windows separators before they reach the archive |
| 7 | Reader strictness | `Store` members only, duplicates rejected, 64 MiB per member | The format is tool-owned; a deflated or oversized member is not a v1 container |
| 8 | Verification | verification happens on parse, not as a separate call on the in-memory value | A container can only be untrustworthy after it was read |
| 9 | API | `New`, `Names`, `Read`, `Text`, `Write`, `WriteText`, `Remove`, `Marshal`, `Parse`, `Load`, `Save` | Minimal surface for S04b, S05 and S08 |
| 10 | `Save` | temp sibling, fsync, chmod 0644, rename; creates the parent directory | Same atomic-write guarantee as the config, no layout knowledge beyond the parent |
| 11 | Fixture | a committed `testdata/container-v1.zip`, generated with `UPDATE_FIXTURES=1` | Catches accidental platform variance in CI on Windows and Linux |
| 12 | Error surface | `store.*` codes with a `store.Error` type | The command layer maps them; no message parsing |

## 2. Error codes

`store.container-corrupt`, `store.container-format`, `store.checksum-missing`,
`store.checksum-mismatch`, `store.checksum-orphan`, `store.reserved-member`, `store.member-name`,
`store.duplicate-member`, `store.member-method`, `store.member-too-large`, `store.write-failed`,
`store.read-failed`.

## 3. Tasks

1. `Container` type with copy-on-read and copy-on-write member handling.
2. Member-name validation and the reserved set.
3. `writeArchive`: fixed header, fixed time, `Store` method, checksum member, sorted order.
4. `Parse`: ZIP read, duplicate and method checks, size limit, meta, checksums, member set match.
5. `Save`/`Load` with the atomic writer.
6. Fixture test with the `UPDATE_FIXTURES` hook.
7. `gofmt`, `go vet`, `go build`, `go test`.

## 4. Acceptance

| S04 criterion | Evidence |
|---|---|
| 1 round-trip is byte-identical | marshal/parse/marshal comparison |
| 2 insertion order does not change the bytes | permuted-write test |
| 3 corrupt, truncated, wrong checksum fail with distinct codes | error table test |
| 4 platform-independent bytes | committed fixture, read on Linux and Windows CI |
| 7 failed write leaves the previous file intact | temp+rename; the code path is exercised by Save/Load |
| 8 two books in one store share no file | no shared state in the codec |
| 9 no working-directory or environment dependency | no such lookup in the package |

## 5. Out of scope

- `_CLABOK` layout, book identity, absolute-path checks and hash pairs: S04b.
- Registry, `diff`, `export`: S04c.
- Open/close, locks, rendering: S05, S06, S09.

## 6. Implementation notes (2026-11-05)

- Members are stored uncompressed. The committed fixture is 1 KB and the store lives outside git, so
  size is not a cost; the bytes no longer depend on the Go flate implementation.
- Member names are validated per path segment. `path.Clean` alone accepts `../evil`, which the
  parse test caught before release; the segment check rejects `.`, `..` and empty segments.
- The reader rejects duplicate entries and any method other than `Store` before it reads data; a
  foreign or deflated member is a format error, not an accepted input.
- Verification happens in `Parse`. There is no `Verify` on the in-memory value, because a container
  can only be untrustworthy after it was read.
- `TestFixture` writes `testdata/container-v1.zip` when `UPDATE_FIXTURES=1` is set and compares the
  bytes otherwise, so a platform-dependent header change fails CI on either OS.
- `Save` creates the parent directory and replaces atomically; a failed write leaves the previous
  container untouched.

