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

// RegistryVersion is the supported library registry schema version. Version 2 added optional
// library defaults; a version 1 document is read as version 2 without defaults.
const RegistryVersion = 2

// Registry kinds.
const (
	KindLibrary = "library"
	KindBook    = "book"
)

// RegistryEntry is one book in the library registry.
//
// Path is the rendered output location, relative to the project directory. The store never
// resolves it; S11 owns the resolution rules. The title and the index file name are deliberately
// not stored: the manifest is the only title source and every book renders a README.md.
type RegistryEntry struct {
	ID   model.BookID `json:"id"`
	Path string       `json:"path"`
	Kind string       `json:"kind"`
}

// Registry is the library registry document. Entry order is the library index order.
type Registry struct {
	Version  int             `json:"version"`
	Books    []RegistryEntry `json:"books"`
	Defaults *model.Settings `json:"defaults,omitempty"`
}

// Library returns the single library entry.
func (registry *Registry) Library() (RegistryEntry, bool) {
	for _, entry := range registry.Books {
		if entry.Kind == KindLibrary {
			return entry, true
		}
	}
	return RegistryEntry{}, false
}

// EffectiveDefaults returns the library defaults with every unset value filled in.
func (registry *Registry) EffectiveDefaults() model.Settings {
	if registry.Defaults == nil {
		return model.DefaultSettings()
	}
	return registry.Defaults.Normalized()
}

// LoadRegistry reads the registry. A missing file yields an empty registry at the current version.
// A version 1 document is upgraded in memory.
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
	registry.Version = RegistryVersion
	return &registry, nil
}

// Validate checks the version and every entry. Versions 1 and 2 are accepted; Save always writes
// the current version.
func (registry *Registry) Validate() error {
	if registry.Version != 1 && registry.Version != RegistryVersion {
		return fail("store.registry-version",
			"unsupported registry version %d (expected %d)", registry.Version, RegistryVersion)
	}
	seenIDs := make(map[string]struct{}, len(registry.Books))
	seenPaths := make(map[string]struct{}, len(registry.Books))
	libraries := 0
	for index, entry := range registry.Books {
		where := "books[" + strconv.Itoa(index) + "]"
		if idErr := ValidateBookID(entry.ID); idErr != nil {
			return fail("store.registry-invalid", "%s.id: %v", where, idErr)
		}
		idKey := BookIDKey(entry.ID)
		if _, duplicate := seenIDs[idKey]; duplicate {
			return fail("store.registry-invalid",
				"%s.id: duplicate book id %q (case-insensitive)", where, string(entry.ID))
		}
		seenIDs[idKey] = struct{}{}
		clean, ok := cleanRegistryPath(entry.Path)
		if !ok {
			return fail("store.registry-invalid",
				"%s.path: expected a relative path inside the project, got %q", where, entry.Path)
		}
		pathKey := PathKey(clean)
		if _, duplicate := seenPaths[pathKey]; duplicate {
			return fail("store.registry-invalid",
				"%s.path: duplicate output path %q (case-insensitive)", where, entry.Path)
		}
		seenPaths[pathKey] = struct{}{}
		switch entry.Kind {
		case KindLibrary:
			libraries++
			if clean != "." {
				return fail("store.registry-invalid",
					"%s.path: the library entry must use \".\", got %q", where, entry.Path)
			}
		case KindBook:
		default:
			return fail("store.registry-invalid",
				"%s.kind: expected %q or %q, got %q", where, KindLibrary, KindBook, entry.Kind)
		}
	}
	if libraries > 1 {
		return fail("store.registry-invalid", "at most one entry may have kind %q", KindLibrary)
	}
	if registry.Defaults != nil {
		defaults := registry.Defaults.Normalized()
		if !defaults.Output.Valid() {
			return fail("store.registry-invalid",
				"defaults.output: unknown output mode %q", string(defaults.Output))
		}
	}
	return nil
}

// cleanRegistryPath validates that a registry path stays inside the project and returns its clean
// form with forward slashes.
func cleanRegistryPath(path string) (string, bool) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" || filepath.IsAbs(trimmed) || strings.HasPrefix(trimmed, "/") ||
		strings.HasPrefix(trimmed, `\`) || (len(trimmed) > 1 && trimmed[1] == ':') {
		return "", false
	}
	if strings.ContainsAny(trimmed, `:*?"<>|`) {
		return "", false
	}
	normalized := filepath.ToSlash(filepath.Clean(trimmed))
	if normalized == ".." || strings.HasPrefix(normalized, "../") {
		return "", false
	}
	for _, segment := range strings.Split(normalized, "/") {
		if segment == "." {
			continue
		}
		if strings.TrimRight(segment, ". ") != segment {
			return "", false
		}
	}
	return normalized, true
}

// RegistryPath validates a path for a new registry entry.
func RegistryPath(path string) (string, error) {
	clean, ok := cleanRegistryPath(path)
	if !ok {
		return "", fail("store.registry-invalid",
			"expected a relative path inside the project, got %q", path)
	}
	return clean, nil
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

// FindFold returns a registry entry by case-insensitive book id. Two ids that differ only by case
// name the same store directory on a case-insensitive file system, so duplicate checks use this.
func (registry *Registry) FindFold(id model.BookID) (RegistryEntry, bool) {
	key := BookIDKey(id)
	for _, entry := range registry.Books {
		if BookIDKey(entry.ID) == key {
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

// Save validates and writes the registry atomically. The library entry is written first; the
// remaining entries keep their relative order.
func (registry *Registry) Save(path string) error {
	registry.Version = RegistryVersion
	if err := registry.Validate(); err != nil {
		return err
	}
	document := struct {
		Version  int             `json:"version"`
		Books    []RegistryEntry `json:"books"`
		Defaults *model.Settings `json:"defaults,omitempty"`
	}{Version: RegistryVersion, Books: orderedEntries(registry.Books), Defaults: registry.Defaults}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return fail("store.write-failed", "%v", err)
	}
	return atomicWrite(path, append(data, '\n'))
}

// orderedEntries places the library entry first and keeps the relative order of the rest.
func orderedEntries(entries []RegistryEntry) []RegistryEntry {
	ordered := make([]RegistryEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Kind == KindLibrary {
			ordered = append(ordered, entry)
		}
	}
	for _, entry := range entries {
		if entry.Kind != KindLibrary {
			ordered = append(ordered, entry)
		}
	}
	return ordered
}

// PeekRegistryVersion reads the raw registry version without upgrading it.
func PeekRegistryVersion(path string) (int, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, fail("store.registry-missing", "%s does not exist", path)
	}
	if err != nil {
		return 0, fail("store.read-failed", "%v", err)
	}
	var header struct {
		Version int `json:"version"`
	}
	if decodeErr := json.Unmarshal(data, &header); decodeErr != nil {
		return 0, fail("store.registry-invalid", "%v", decodeErr)
	}
	return header.Version, nil
}
