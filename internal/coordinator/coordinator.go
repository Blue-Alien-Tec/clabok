// Package coordinator serializes writers on book resources.
//
// The contract: one writer per locked resource, acquisitions in a stable order (book, node, block)
// so crossing operations cannot deadlock, and reads without a write lock. A reader relies on the
// store's atomic file replacement and never observes a half-written member.
//
// The file backend uses operating-system advisory locks, which the OS releases when a process dies.
// There is no stale-lock cleanup and no lock-file deletion race.
package coordinator

import (
	"fmt"
	"sort"
	"time"

	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

// DefaultTimeout bounds a lock wait when the caller passes no timeout.
const DefaultTimeout = 5 * time.Second

// Kind is the granularity of a locked resource.
type Kind string

const (
	// KindStore locks a store-level file shared by all books, such as the open state.
	KindStore Kind = "store"
	// KindBook locks a whole book structure.
	KindBook Kind = "book"
	// KindNode locks one structural node.
	KindNode Kind = "node"
	// KindBlock locks one block body.
	KindBlock Kind = "block"
)

// Resource identifies one lockable unit.
type Resource struct {
	Kind Kind
	Book model.BookID
	ID   model.ID
}

// String renders the resource for messages and lock-file names.
func (resource Resource) String() string {
	switch resource.Kind {
	case KindStore:
		return "store"
	case KindBook:
		return "book:" + string(resource.Book)
	case KindNode:
		return "node:" + string(resource.Book) + ":" + resource.ID.String()
	default:
		return "block:" + string(resource.Book) + ":" + resource.ID.String()
	}
}

func (resource Resource) validate() error {
	switch resource.Kind {
	case KindStore:
		if resource.Book != "" || !resource.ID.IsZero() {
			return fail("coordinator.invalid-resource", "a store resource carries no book and no id")
		}
	case KindBook:
		if !resource.Book.Valid() {
			return fail("coordinator.invalid-resource", "a book resource needs a valid book id")
		}
		if !resource.ID.IsZero() {
			return fail("coordinator.invalid-resource", "a book resource carries no id")
		}
	case KindNode, KindBlock:
		if !resource.Book.Valid() {
			return fail("coordinator.invalid-resource", "%s resource needs a valid book id", resource.Kind)
		}
		if resource.ID.IsZero() {
			return fail("coordinator.invalid-resource", "%s resource needs a non-zero id", resource.Kind)
		}
	default:
		return fail("coordinator.invalid-resource", "unknown resource kind %q", resource.Kind)
	}
	return nil
}

func (resource Resource) rank() int {
	switch resource.Kind {
	case KindStore:
		return 0
	case KindBook:
		return 1
	case KindNode:
		return 2
	default:
		return 3
	}
}

func (resource Resource) less(other Resource) bool {
	if resource.rank() != other.rank() {
		return resource.rank() < other.rank()
	}
	if resource.Book != other.Book {
		return resource.Book < other.Book
	}
	return resource.ID < other.ID
}

func (resource Resource) lockName() string {
	switch resource.Kind {
	case KindStore:
		return "store.lock"
	case KindBook:
		return "book-" + string(resource.Book) + ".lock"
	case KindNode:
		return "node-" + string(resource.Book) + "-" + resource.ID.String() + ".lock"
	default:
		return "block-" + string(resource.Book) + "-" + resource.ID.String() + ".lock"
	}
}

// sortResources validates, deduplicates and orders a resource set.
func sortResources(resources []Resource) ([]Resource, error) {
	seen := make(map[string]struct{}, len(resources))
	ordered := make([]Resource, 0, len(resources))
	for _, resource := range resources {
		if err := resource.validate(); err != nil {
			return nil, err
		}
		key := resource.String()
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		ordered = append(ordered, resource)
	}
	sort.Slice(ordered, func(left, right int) bool {
		return ordered[left].less(ordered[right])
	})
	return ordered, nil
}

// Mode is the lock mode.
type Mode int

const (
	// Exclusive grants one holder and excludes shared holders.
	Exclusive Mode = iota
	// Shared allows several holders and excludes an exclusive holder.
	Shared
)

// Release frees the acquired locks. It is safe to call more than once.
type Release func() error

// Coordinator serializes writers. The implementation must be interchangeable: file locks and the
// daemon mutexes provide the same semantics.
type Coordinator interface {
	Acquire(resources []Resource, mode Mode, timeout time.Duration) (Release, error)
}

// Error is a coordinator failure with a stable code.
type Error struct {
	Code    string
	Message string
}

// Error returns the human message.
func (err *Error) Error() string { return err.Message }

// ErrorCode returns the stable code for the CLI boundary.
func (err *Error) ErrorCode() string { return err.Code }

func fail(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}
