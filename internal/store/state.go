package store

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Status is the drift state of one book.
type Status string

const (
	// StatusClean means the stored input and output hashes match the current ones.
	StatusClean Status = "clean"
	// StatusDirtyInput means the source state changed since the last recorded render.
	StatusDirtyInput Status = "dirty-input"
	// StatusDirtyOutput means the generated output changed since the last recorded render.
	StatusDirtyOutput Status = "dirty-output"
	// StatusDirtyBoth means both sides changed.
	StatusDirtyBoth Status = "dirty-both"
	// StatusNoState means no state has been recorded for the book.
	StatusNoState Status = "no-state"
)

// State is the recorded input and output hash of one book.
type State struct {
	InputHash  string `json:"inputHash"`
	OutputHash string `json:"outputHash"`
}

// Compare reports the drift for the given current hashes.
func (state State) Compare(input, output string) Status {
	if state.InputHash == "" && state.OutputHash == "" {
		return StatusNoState
	}
	dirtyInput := state.InputHash != input
	dirtyOutput := state.OutputHash != output
	switch {
	case dirtyInput && dirtyOutput:
		return StatusDirtyBoth
	case dirtyInput:
		return StatusDirtyInput
	case dirtyOutput:
		return StatusDirtyOutput
	}
	return StatusClean
}

// HashMembers hashes a member set deterministically. Each member contributes its name, a NUL
// separator, its length and its data, in sorted name order.
func HashMembers(members map[string][]byte) string {
	hash := sha256.New()
	var length [8]byte
	for _, name := range sortedNames(members) {
		data := members[name]
		hash.Write([]byte(name))
		hash.Write([]byte{0})
		binary.LittleEndian.PutUint64(length[:], uint64(len(data)))
		hash.Write(length[:])
		hash.Write(data)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// HashBytes hashes raw bytes, for a container file or any other single blob.
func HashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// ReadState reads the recorded state. The boolean reports whether a state file exists.
func (paths BookPaths) ReadState() (State, bool, error) {
	data, err := os.ReadFile(paths.StateFile)
	if errors.Is(err, os.ErrNotExist) {
		return State{}, false, nil
	}
	if err != nil {
		return State{}, false, fail("store.read-failed", "%v", err)
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, false, fail("store.state-corrupt", "%s: %v", paths.StateFile, err)
	}
	return state, true, nil
}

// WriteState records the state atomically. A state with both hashes empty removes the file.
func (paths BookPaths) WriteState(state State) error {
	if state.InputHash == "" && state.OutputHash == "" {
		err := os.Remove(paths.StateFile)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fail("store.write-failed", "%v", err)
		}
		return nil
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fail("store.write-failed", "%v", err)
	}
	return atomicWrite(paths.StateFile, append(data, '\n'))
}

// OpenBookCount reports how many books the open state file lists.
func OpenBookCount(path string) (int, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fail("store.read-failed", "%v", err)
	}
	var file struct {
		Version int `json:"version"`
		Books   []struct {
			ID string `json:"id"`
		} `json:"books"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return 0, fail("store.state-corrupt", "%s: %v", path, err)
	}
	return len(file.Books), nil
}

// WriteAtomic replaces a file through a temp sibling in the same directory. Use it for store-level
// state files that are not container members.
func WriteAtomic(path string, data []byte) error {
	return atomicWrite(path, data)
}

// atomicWrite replaces a file through a temp sibling in the same directory.
func atomicWrite(path string, data []byte) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fail("store.write-failed", "%v", err)
	}
	temporary, err := os.CreateTemp(directory, ".store-*.tmp")
	if err != nil {
		return fail("store.write-failed", "%v", err)
	}
	name := temporary.Name()
	discard := func() {
		_ = temporary.Close()
		_ = os.Remove(name)
	}
	if _, err := temporary.Write(data); err != nil {
		discard()
		return fail("store.write-failed", "%v", err)
	}
	if err := temporary.Sync(); err != nil {
		discard()
		return fail("store.write-failed", "%v", err)
	}
	if err := temporary.Close(); err != nil {
		_ = os.Remove(name)
		return fail("store.write-failed", "%v", err)
	}
	if err := os.Chmod(name, 0o644); err != nil {
		_ = os.Remove(name)
		return fail("store.write-failed", "%v", err)
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return fail("store.write-failed", "%v", err)
	}
	return nil
}
