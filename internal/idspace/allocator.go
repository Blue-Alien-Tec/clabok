package idspace

import (
	"errors"
	"math"
	"os"

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

// Reserve advances the ledger once and returns the first ID of a consecutive range. It is the
// import path: one save for many IDs instead of one per ID.
func (allocator *Allocator) Reserve(count int) (model.ID, error) {
	if count <= 0 {
		return model.ZeroID, fail("idspace.invalid-count", "reserve needs a positive count")
	}
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
	base := ledger.Next
	if base.IsZero() {
		base = 1
	}
	if uint64(base)+uint64(count) >= math.MaxUint64 {
		return model.ZeroID, fail("idspace.exhausted", "the ID space is exhausted")
	}
	ledger.Next = base + model.ID(count)
	if saveErr := allocator.saveLocked(ledger); saveErr != nil {
		return model.ZeroID, saveErr
	}
	return base, nil
}

// EnsureAbove advances the ledger past id. A restored container may carry IDs above the ledger.
func (allocator *Allocator) EnsureAbove(id model.ID) error {
	if allocator == nil || allocator.Life == nil {
		return fail("idspace.not-configured", "the ID allocator has no lifecycle")
	}
	release, err := allocator.Life.LockIDs()
	if err != nil {
		return err
	}
	defer func() { _ = release() }()
	ledger, err := allocator.loadLocked()
	if err != nil {
		return err
	}
	if ledger.Next > id {
		return nil
	}
	ledger.Next = id + 1
	return allocator.saveLocked(ledger)
}

// SnapshotLedger returns the raw ledger bytes for a rollback; nil means the file does not exist.
func (allocator *Allocator) SnapshotLedger() ([]byte, error) {
	if allocator == nil || allocator.Life == nil {
		return nil, fail("idspace.not-configured", "the ID allocator has no lifecycle")
	}
	release, err := allocator.Life.LockIDs()
	if err != nil {
		return nil, err
	}
	defer func() { _ = release() }()
	data, readErr := os.ReadFile(allocator.Layout.IDs())
	if errors.Is(readErr, os.ErrNotExist) {
		return nil, nil
	}
	if readErr != nil {
		return nil, fail("idspace.read-failed", "%v", readErr)
	}
	return data, nil
}

// RestoreLedger writes ledger bytes back; nil removes the ledger so the next allocation rescans.
func (allocator *Allocator) RestoreLedger(data []byte) error {
	if allocator == nil || allocator.Life == nil {
		return fail("idspace.not-configured", "the ID allocator has no lifecycle")
	}
	release, err := allocator.Life.LockIDs()
	if err != nil {
		return err
	}
	defer func() { _ = release() }()
	if data == nil {
		if removeErr := os.Remove(allocator.Layout.IDs()); removeErr != nil &&
			!errors.Is(removeErr, os.ErrNotExist) {
			return fail("idspace.write-failed", "%v", removeErr)
		}
		return nil
	}
	return store.WriteAtomic(allocator.Layout.IDs(), data)
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
