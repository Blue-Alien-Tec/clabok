package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

// RegistryVersion is the supported library registry schema version.
const RegistryVersion = 1

// Registry kinds.
const (
	KindLibrary = "library"
	KindBook    = "book"
)

// RegistryEntry is one book in the library registry.
//
// Path is the rendered output location, relative to the project directory. The store never
// resolves it; S11 owns the resolution rules.
type RegistryEntry struct {
	ID   model.BookID `json:"id"`
	Path string       `json:"path"`
	Kind string       `json:"kind"`
}

// Registry is the library registry document. Entry order is the library index order.
type Registry struct {
	Version int             `json:"version"`
	Books   []RegistryEntry `json:"books"`
}

// LoadRegistry reads the registry. A missing file yields an empty registry at the current version.
func LoadRegistry(path string) (*Registry, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Registry{Version: RegistryVersion}, nil
	}
	if err != nil {
		return nil, fail("store.read-failed", "%v", err)
	}
	var registry Registry
	if err := json.Unmarshal(data, &registry); err != nil {
		return nil, fail("store.registry-corrupt", "%s: %v", path, err)
	}
	if err := registry.Validate(); err != nil {
		return nil, err
	}
	return &registry, nil
}

// Validate checks the version and every entry.
func (registry *Registry) Validate() error {
	if registry.Version != RegistryVersion {
		return fail("store.registry-version",
			"unsupported registry version %d (expected %d)", registry.Version, RegistryVersion)
	}
	seen := make(map[model.BookID]struct{}, len(registry.Books))
	for index, entry := range registry.Books {
		where := "books[" + strconv.Itoa(index) + "]"
		if !entry.ID.Valid() {
			return fail("store.registry-invalid", "%s.id: invalid book id %q", where, string(entry.ID))
		}
		if _, duplicate := seen[entry.ID]; duplicate {
			return fail("store.registry-invalid", "%s.id: duplicate book id %q", where, string(entry.ID))
		}
		seen[entry.ID] = struct{}{}
		if strings.TrimSpace(entry.Path) == "" {
			return fail("store.registry-invalid", "%s.path: must not be empty", where)
		}
		if filepath.IsAbs(entry.Path) {
			return fail("store.registry-invalid", "%s.path: must be relative, got %q", where, entry.Path)
		}
		if entry.Kind != KindLibrary && entry.Kind != KindBook {
			return fail("store.registry-invalid",
				"%s.kind: expected %q or %q, got %q", where, KindLibrary, KindBook, entry.Kind)
		}
	}
	return nil
}

// Find returns a registry entry by book ID.
func (registry *Registry) Find(id model.BookID) (RegistryEntry, bool) {
	for _, entry := range registry.Books {
		if entry.ID == id {
			return entry, true
		}
	}
	return RegistryEntry{}, false
}

// Upsert inserts or replaces an entry, keeping the position of an existing one.
func (registry *Registry) Upsert(entry RegistryEntry) {
	for index := range registry.Books {
		if registry.Books[index].ID == entry.ID {
			registry.Books[index] = entry
			return
		}
	}
	registry.Books = append(registry.Books, entry)
}

// Remove deletes an entry and reports whether it existed.
func (registry *Registry) Remove(id model.BookID) bool {
	for index, entry := range registry.Books {
		if entry.ID == id {
			registry.Books = append(registry.Books[:index], registry.Books[index+1:]...)
			return true
		}
	}
	return false
}

// Save validates and writes the registry atomically.
func (registry *Registry) Save(path string) error {
	registry.Version = RegistryVersion
	if err := registry.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return fail("store.write-failed", "%v", err)
	}
	return atomicWrite(path, append(data, '\n'))
}
