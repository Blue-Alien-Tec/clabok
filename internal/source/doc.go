// Package source is the single read-only view of a CLABOK book.
//
// A book is read either from the extracted state (open or unsaved) or from the packed container
// (closed). Both paths produce the same Book value, so a consumer that only reads never needs to
// branch on the open state. No lock is taken: every store write is atomic, so a reader always sees
// complete files. Consistency across several reads requires the caller to hold the book lock.
package source
