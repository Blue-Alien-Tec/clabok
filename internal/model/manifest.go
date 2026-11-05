package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/Blue-Alien-Tec/clabok/internal/jsonstrict"
)

// Version is the supported manifest schema version.
const Version = 1

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

// Manifest is the structure document of one book. Bodies are not part of it: they live in the body
// store and in Book.Bodies.
type Manifest struct {
	Version  int       `json:"version"`
	ID       BookID    `json:"id"`
	Root     *Node     `json:"root"`
	Regions  []*Region `json:"regions,omitempty"`
	Settings Settings  `json:"settings"`
}

// Error is a manifest-level failure with a stable code.
type Error struct {
	Code    string
	Message string
}

// Error returns the human message.
func (err *Error) Error() string { return err.Message }

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
	if manifest.Version != Version {
		if manifest.Version > Version {
			return nil, &Error{
				Code: "model.manifest-version",
				Message: fmt.Sprintf("manifest version %d is newer than pecia supports (%d)",
					manifest.Version, Version),
			}
		}
		return nil, &Error{
			Code: "model.manifest-version",
			Message: fmt.Sprintf("unsupported manifest version %d (expected %d)",
				manifest.Version, Version),
		}
	}
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
