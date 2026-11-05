package idspace

import (
	"math"

	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

// Allocator issues globally unique IDs from the library ledger.
type Allocator struct {
	Life   *lifecycle.Lifecycle
	Layout store.Layout
}

// New returns the library ID allocator. It satisfies model.Allocator.
func New(life *lifecycle.Lifecycle, layout store.Layout) *Allocator {
	return &Allocator{Life: life, Layout: layout}
}

// Next returns the next globally unique ID and persists the advanced ledger.
func (allocator *Allocator) Next() (model.ID, error) {
	if allocator == nil || allocator.Life == nil {
		return model.ZeroID, fail("idspace.not-configured", "the ID allocator has no lifecycle")
	}
	release, err := allocator.Life.LockIDs()
	if err != nil {
		return model.ZeroID, err
	}
	defer func() { _ = release() }()
	ledger, err := allocator.loadLocked()
	if err != nil {
		return model.ZeroID, err
	}
	id := ledger.Next
	if id.IsZero() {
		id = 1
	}
	if uint64(id) == math.MaxUint64 {
		return model.ZeroID, fail("idspace.exhausted", "the ID space is exhausted")
	}
	ledger.Next = id + 1
	if saveErr := allocator.saveLocked(ledger); saveErr != nil {
		return model.ZeroID, saveErr
	}
	return id, nil
}

// Peek returns the next ID without advancing the ledger. It is used by dry runs, which must not
// write.
func (allocator *Allocator) Peek() (model.ID, error) {
	if allocator == nil || allocator.Life == nil {
		return model.ZeroID, fail("idspace.not-configured", "the ID allocator has no lifecycle")
	}
	release, err := allocator.Life.LockIDs()
	if err != nil {
		return model.ZeroID, err
	}
	defer func() { _ = release() }()
	ledger, err := allocator.loadLocked()
	if err != nil {
		return model.ZeroID, err
	}
	if ledger.Next.IsZero() {
		return 1, nil
	}
	return ledger.Next, nil
}
