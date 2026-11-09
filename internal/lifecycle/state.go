package lifecycle

import (
	"encoding/json"
	"errors"
	"os"

	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

// StateVersion is the supported open-state schema version.
const StateVersion = 1

type openFile struct {
	Version int     `json:"version"`
	Books   []Entry `json:"books"`
}

func (file *openFile) find(id model.BookID) (Entry, bool) {
	for _, entry := range file.Books {
		if entry.ID == id {
			return entry, true
		}
	}
	return Entry{}, false
}

func (file *openFile) upsert(entry Entry) {
	for index := range file.Books {
		if file.Books[index].ID == entry.ID {
			file.Books[index] = entry
			return
		}
	}
	file.Books = append(file.Books, entry)
}

func (file *openFile) remove(id model.BookID) bool {
	for index, entry := range file.Books {
		if entry.ID == id {
			file.Books = append(file.Books[:index], file.Books[index+1:]...)
			return true
		}
	}
	return false
}

func (life *Lifecycle) readOpenFile() (*openFile, error) {
	path := life.layout.OpenState()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &openFile{Version: StateVersion}, nil
	}
	if err != nil {
		return nil, fail("lifecycle.read-failed", "%v", err)
	}
	var file openFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fail("lifecycle.state-corrupt", "%s: %v", path, err)
	}
	if file.Version != StateVersion {
		return nil, fail("lifecycle.state-version",
			"unsupported open state version %d (expected %d)", file.Version, StateVersion)
	}
	return &file, nil
}

func (life *Lifecycle) writeOpenFile(file *openFile) error {
	path := life.layout.OpenState()
	file.Version = StateVersion
	if len(file.Books) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fail("lifecycle.write-failed", "%v", err)
		}
		return nil
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fail("lifecycle.write-failed", "%v", err)
	}
	if err := store.WriteAtomic(path, append(data, '\n')); err != nil {
		return err
	}
	return nil
}

// PeekStateVersion reads the raw open-state version. A missing file is version 0.
func PeekStateVersion(path string) (int, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fail("lifecycle.read-failed", "%v", err)
	}
	var header struct {
		Version int `json:"version"`
	}
	if decodeErr := json.Unmarshal(data, &header); decodeErr != nil {
		return 0, fail("lifecycle.state-corrupt", "%v", decodeErr)
	}
	return header.Version, nil
}
