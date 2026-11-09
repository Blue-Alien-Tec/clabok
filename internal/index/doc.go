// Package index owns the persistent derived search index.
//
// The index is a cache, never the source of truth: it is built from closed books only, keyed by a
// container fingerprint, and rebuilt when the container changes. It is stored under <store>/index
// with one file per book and a global manifest that records the format version and the fingerprints.
// Every write is atomic and the file content is deterministic for the same input.
package index
