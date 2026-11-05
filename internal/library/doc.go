// Package library owns the library registry: its defaults, the books registered in it, the
// generated Document Index, and the book lifecycle commands that act on the registry.
//
// The library never owns rendering. It resolves registry data and delegates every render to the
// render package, so a book renders byte-identically whether the library or the sync command drives
// it.
package library
