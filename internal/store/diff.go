package store

import (
	"bytes"
	"path/filepath"
	"sort"
	"strings"
)

// ChangeKind classifies a member difference between two container states.
type ChangeKind string

const (
	// ChangeAdded is a member that exists only in the later state.
	ChangeAdded ChangeKind = "added"
	// ChangeChanged is a member whose bytes differ.
	ChangeChanged ChangeKind = "changed"
	// ChangeRemoved is a member that exists only in the earlier state.
	ChangeRemoved ChangeKind = "removed"
)

// Change is one member difference.
type Change struct {
	Name string     `json:"name"`
	Kind ChangeKind `json:"kind"`
}

// Diff reports the member differences from before to after, sorted by member name.
func Diff(before, after *Container) []Change {
	names := make(map[string]struct{}, len(before.members)+len(after.members))
	for name := range before.members {
		names[name] = struct{}{}
	}
	for name := range after.members {
		names[name] = struct{}{}
	}
	ordered := make([]string, 0, len(names))
	for name := range names {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)

	var changes []Change
	for _, name := range ordered {
		beforeData, beforeExists := before.members[name]
		afterData, afterExists := after.members[name]
		switch {
		case !beforeExists && afterExists:
			changes = append(changes, Change{Name: name, Kind: ChangeAdded})
		case beforeExists && !afterExists:
			changes = append(changes, Change{Name: name, Kind: ChangeRemoved})
		case beforeExists && afterExists && !bytes.Equal(beforeData, afterData):
			changes = append(changes, Change{Name: name, Kind: ChangeChanged})
		}
	}
	return changes
}

// Export writes every member into directory, recreating the member tree. It returns the written
// member names in sorted order.
func (container *Container) Export(directory string) ([]string, error) {
	if strings.TrimSpace(directory) == "" {
		return nil, fail("store.invalid-root", "the export directory must not be empty")
	}
	root, err := filepath.Abs(directory)
	if err != nil {
		return nil, fail("store.invalid-root", "%v", err)
	}
	names := container.Names()
	written := make([]string, 0, len(names))
	for _, name := range names {
		target := filepath.Join(root, filepath.FromSlash(name))
		if !within(root, target) {
			return nil, fail("store.member-name", "member %q escapes the export directory", name)
		}
		if err := atomicWrite(target, container.members[name]); err != nil {
			return nil, err
		}
		written = append(written, name)
	}
	return written, nil
}

func within(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
