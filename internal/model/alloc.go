package model

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"sync"
)

// Allocator issues IDs. The model never allocates an ID on its own; the caller supplies the
// allocator, so the library can switch to a registry-backed sequence without touching the model.
type Allocator interface {
	Next() (ID, error)
}

// Sequence allocates consecutive IDs from a start value.
type Sequence struct {
	mutex sync.Mutex
	next  uint64
}

// NewSequenceAllocator starts a sequence at start. Zero is skipped.
func NewSequenceAllocator(start ID) *Sequence {
	value := uint64(start)
	if value == 0 {
		value = 1
	}
	return &Sequence{next: value}
}

// Next returns the next ID.
func (sequence *Sequence) Next() (ID, error) {
	sequence.mutex.Lock()
	defer sequence.mutex.Unlock()
	if sequence.next == 0 {
		sequence.next = 1
	}
	value := ID(sequence.next)
	sequence.next++
	if sequence.next == 0 {
		sequence.next = 1
	}
	return value, nil
}

// Random allocates 63-bit random IDs. It needs no coordination, so two independent processes never
// collide in practice; the library may still prefer a sequential allocator for readability.
type Random struct{}

// NewRandomAllocator returns the cooperative default allocator.
func NewRandomAllocator() Random { return Random{} }

// Next returns a nonzero random ID.
func (Random) Next() (ID, error) {
	var buffer [8]byte
	if _, err := rand.Read(buffer[:]); err != nil {
		return ZeroID, errors.New("model: cannot read random bytes: " + err.Error())
	}
	value := binary.LittleEndian.Uint64(buffer[:]) & (1<<63 - 1)
	if value == 0 {
		value = 1
	}
	return ID(value), nil
}
