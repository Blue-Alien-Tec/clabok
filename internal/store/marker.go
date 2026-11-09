package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// StoreVersion is the version of the store layout marker. A missing marker reads as version 0.
const StoreVersion = 1

const (
	storeFile     = "store.json"
	migrationsDir = "migrations"
)

// StoreMarker is the store layout marker of one store root.
type StoreMarker struct {
	Version int `json:"version"`
}

// StoreMarkerPath returns the marker file of the store root.
func (layout Layout) StoreMarkerPath() string { return filepath.Join(layout.Root, storeFile) }

// MigrationsRoot holds the migration history and the store backups.
func (layout Layout) MigrationsRoot() string { return filepath.Join(layout.Root, migrationsDir) }

// PeekStoreVersion reads the raw marker version without upgrading it. A missing marker is 0.
func PeekStoreVersion(path string) (int, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fail("store.read-failed", "%v", err)
	}
	var marker StoreMarker
	if decodeErr := json.Unmarshal(data, &marker); decodeErr != nil {
		return 0, fail("store.invalid-marker", "%v", decodeErr)
	}
	return marker.Version, nil
}

// WriteStoreVersion writes the marker atomically.
func WriteStoreVersion(path string, version int) error {
	data, err := json.MarshalIndent(StoreMarker{Version: version}, "", "  ")
	if err != nil {
		return fail("store.write-failed", "%v", err)
	}
	if err := WriteAtomic(path, append(data, '\n')); err != nil {
		return fail("store.write-failed", "%v", err)
	}
	return nil
}
