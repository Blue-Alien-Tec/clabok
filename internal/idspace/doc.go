// Package idspace is the library's authority for the global ID namespace.
//
// PLAN.md §3.12: IDs are library-managed and unique across all books; global uniqueness is a
// deliberate constraint, not a derived property. The allocator keeps that promise with one
// persisted monotonic ledger (ids.json) guarded by the dedicated KindIDs lock. Every ID issued by
// the tool comes from here. IDs are immutable: a move or reparent changes only the ordered
// collections, and a copy or import allocates fresh IDs instead of changing an existing one.
//
// Lock discipline: the allocator acquires only the ID lock and never a store or book lock. It is
// therefore safe to call while a book lock is held. A multi-lock operation must never hold the ID
// lock while waiting for a book.
package idspace
