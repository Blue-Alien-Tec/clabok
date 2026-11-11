package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/Blue-Alien-Tec/clabok/internal/jsonstrict"
)

// Version is the supported manifest schema version. Version 2 added optional block metadata;
// version 3 added the index-prose block reference; version 4 added the optional book language;
// version 5 added the optional library README layout.
const Version = 5

// BookID names a book in the store. It is not an element ID and is not referenced by other books.
type BookID string

// Valid reports whether the book ID can name a store folder.
func (id BookID) Valid() bool {
	text := string(id)
	if text == "" || text == "." || text == ".." {
		return false
	}
	if strings.ContainsAny(text, `/\`) {
		return false
	}
	for _, character := range text {
		if unicode.IsSpace(character) || unicode.IsControl(character) {
			return false
		}
	}
	return true
}

// BlockMeta is the optional metadata of one placed block.
type BlockMeta struct {
	// Type is a free-form token for later search filters, for example UILayout.
	Type string `json:"type,omitempty"`
	// Locked protects the generated content: a command may not change the body or the type.
	Locked bool `json:"locked,omitempty"`
	// Status is the assigned editorial state while the book enables the status system.
	Status string `json:"status,omitempty"`
}

// Manifest is the structure document of one book. Bodies are not part of it: they live in the body
// store and in Book.Bodies.
type Manifest struct {
	Version   int              `json:"version"`
	ID        BookID           `json:"id"`
	Root      *Node            `json:"root"`
	Regions   []*Region        `json:"regions,omitempty"`
	BlockMeta map[ID]BlockMeta `json:"blockMeta,omitempty"`
	Status    *StatusConfig    `json:"status,omitempty"`
	Badges    []Badge          `json:"badges,omitempty"`
	Layout    *LayoutConfig    `json:"layout,omitempty"`
	Settings  Settings         `json:"settings"`
}

// Meta returns the metadata of a block, or the zero value.
func (manifest *Manifest) Meta(id ID) BlockMeta {
	if manifest.BlockMeta == nil {
		return BlockMeta{}
	}
	return manifest.BlockMeta[id]
}

// SetMeta writes block metadata. The zero value removes the entry.
func (manifest *Manifest) SetMeta(id ID, meta BlockMeta) {
	if meta.Type == "" && !meta.Locked && meta.Status == "" {
		delete(manifest.BlockMeta, id)
		if len(manifest.BlockMeta) == 0 {
			manifest.BlockMeta = nil
		}
		return
	}
	if manifest.BlockMeta == nil {
		manifest.BlockMeta = map[ID]BlockMeta{}
	}
	manifest.BlockMeta[id] = meta
}

// IsLocked reports whether the block content is protected.
func (manifest *Manifest) IsLocked(id ID) bool { return manifest.Meta(id).Locked }

// BlockType returns the optional type token of a block.
func (manifest *Manifest) BlockType(id ID) string { return manifest.Meta(id).Type }

// Error is a manifest-level failure with a stable code.
type Error struct {
	Code    string
	Message string
}

// Error returns the human message.
func (err *Error) Error() string { return err.Message }

// ErrorCode returns the stable code for the CLI boundary.
func (err *Error) ErrorCode() string { return err.Code }

// NewManifest creates an empty book with a title root.
func NewManifest(id BookID, title string, allocator Allocator) (*Manifest, error) {
	if !id.Valid() {
		return nil, &Error{Code: "model.invalid-book-id", Message: fmt.Sprintf("invalid book id %q", string(id))}
	}
	if allocator == nil {
		return nil, &Error{Code: "model.no-allocator", Message: "no id allocator supplied"}
	}
	rootID, err := allocator.Next()
	if err != nil {
		return nil, &Error{Code: "model.alloc-failed", Message: err.Error()}
	}
	return &Manifest{
		Version: Version,
		ID:      id,
		Root: &Node{
			ID:    rootID,
			Kind:  KindTitle,
			Title: NormalizeTitle(title),
		},
		Settings: DefaultSettings(),
	}, nil
}

// Decode reads and normalizes a manifest document.
func Decode(data []byte) (*Manifest, error) {
	var manifest Manifest
	err := jsonstrict.Decode(data, &manifest, jsonstrict.Options{DisallowUnknownFields: true})
	if err != nil {
		var strict *jsonstrict.Error
		if errors.As(err, &strict) {
			switch strict.Kind {
			case jsonstrict.KindUnknownField:
				return nil, &Error{Code: "model.unknown-key", Message: strict.Message}
			case jsonstrict.KindDuplicateKey:
				return nil, &Error{Code: "model.duplicate-key", Message: strict.Message}
			}
		}
		return nil, &Error{Code: "model.invalid-json", Message: err.Error()}
	}
	if manifest.Version > Version {
		return nil, &Error{
			Code: "model.manifest-version",
			Message: fmt.Sprintf("manifest version %d is newer than pecia supports (%d)",
				manifest.Version, Version),
		}
	}
	if manifest.Version < 2 {
		return nil, &Error{
			Code: "model.manifest-version",
			Message: fmt.Sprintf("unsupported manifest version %d (expected 2 or %d)",
				manifest.Version, Version),
		}
	}
	manifest.Version = Version
	manifest.Normalize()
	return &manifest, nil
}

// Encode serializes the manifest deterministically.
func (manifest *Manifest) Encode() ([]byte, error) {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// Normalize fills the defaults that a zero value cannot express.
func (manifest *Manifest) Normalize() {
	manifest.Settings.normalize()
}

// Clone returns a deep copy.
func (manifest *Manifest) Clone() *Manifest {
	if manifest == nil {
		return nil
	}
	clone := &Manifest{
		Version:  manifest.Version,
		ID:       manifest.ID,
		Root:     manifest.Root.Clone(),
		Settings: manifest.Settings,
	}
	if manifest.Status != nil {
		status := *manifest.Status
		clone.Status = &status
	}
	if manifest.Badges != nil {
		clone.Badges = make([]Badge, len(manifest.Badges))
		copy(clone.Badges, manifest.Badges)
	}
	clone.Layout = manifest.Layout.Clone()
	if manifest.BlockMeta != nil {
		clone.BlockMeta = make(map[ID]BlockMeta, len(manifest.BlockMeta))
		for id, meta := range manifest.BlockMeta {
			clone.BlockMeta[id] = meta
		}
	}
	if manifest.Regions != nil {
		clone.Regions = make([]*Region, len(manifest.Regions))
		for index, region := range manifest.Regions {
			clone.Regions[index] = region.Clone()
		}
	}
	return clone
}

// Walk visits the structure depth-first.
func (manifest *Manifest) Walk(visit func(current *Node, depth int, parent *Node) bool) {
	if manifest == nil || manifest.Root == nil {
		return
	}
	manifest.Root.Walk(visit)
}

// FindNode returns a node by ID.
func (manifest *Manifest) FindNode(id ID) (*Node, bool) {
	var found *Node
	manifest.Walk(func(current *Node, depth int, parent *Node) bool {
		if current.ID == id {
			found = current
			return false
		}
		return found == nil
	})
	return found, found != nil
}

// PlacedBlocks returns every placed block ID in structure order, then in region order.
func (manifest *Manifest) PlacedBlocks() []ID {
	var ids []ID
	manifest.Walk(func(current *Node, depth int, parent *Node) bool {
		ids = append(ids, current.Blocks...)
		return true
	})
	for _, region := range manifest.Regions {
		if region != nil {
			ids = append(ids, region.Blocks...)
		}
	}
	return ids
}

// OwnedIDs returns every ID the manifest owns, sorted ascending: nodes, placed blocks, regions,
// region targets and block metadata. Zero is not owned. The list may contain duplicates; the
// validator rejects a duplicate ownership inside one book.
func (manifest *Manifest) OwnedIDs() []ID {
	ids := make([]ID, 0)
	appendID := func(id ID) {
		if !id.IsZero() {
			ids = append(ids, id)
		}
	}
	manifest.Walk(func(current *Node, depth int, parent *Node) bool {
		appendID(current.ID)
		for _, block := range current.Blocks {
			appendID(block)
		}
		return true
	})
	for _, region := range manifest.Regions {
		if region == nil {
			continue
		}
		appendID(region.ID)
		appendID(region.Target.ID)
		for _, block := range region.Blocks {
			appendID(block)
		}
	}
	for id := range manifest.BlockMeta {
		appendID(id)
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	return ids
}

// IsPlaced reports whether a block ID has a placement.
func (manifest *Manifest) IsPlaced(id ID) bool {
	if manifest == nil {
		return false
	}
	for _, placed := range manifest.PlacedBlocks() {
		if placed == id {
			return true
		}
	}
	return false
}
