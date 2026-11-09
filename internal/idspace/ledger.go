package idspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

// LedgerVersion is the supported ID ledger schema version.
const LedgerVersion = 1

// Ledger is the persisted ID namespace. Next is the next ID to issue; zero means the ledger has
// not been initialized and is rebuilt from the registered books.
type Ledger struct {
	Version int      `json:"version"`
	Next    model.ID `json:"next"`
}

// Error is a coded idspace error.
type Error struct {
	Code    string
	Message string
}

// Error renders the coded message.
func (err *Error) Error() string { return err.Code + ": " + err.Message }

// ErrorCode returns the stable error code.
func (err *Error) ErrorCode() string { return err.Code }

func fail(code, format string, args ...any) error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// loadLocked reads the ledger. The caller holds the ID lock. A missing ledger is initialized from
// the highest owned ID of every registered book, so upgrading a store that predates the ledger
// cannot collide with existing IDs.
func (allocator *Allocator) loadLocked() (Ledger, error) {
	data, err := os.ReadFile(allocator.Layout.IDs())
	if errors.Is(err, os.ErrNotExist) {
		next, scanErr := allocator.scanNext()
		if scanErr != nil {
			return Ledger{}, scanErr
		}
		return Ledger{Version: LedgerVersion, Next: next}, nil
	}
	if err != nil {
		return Ledger{}, fail("idspace.read-failed", "%v", err)
	}
	var ledger Ledger
	if err := json.Unmarshal(data, &ledger); err != nil {
		return Ledger{}, fail("idspace.ledger-corrupt", "%s: %v", allocator.Layout.IDs(), err)
	}
	if ledger.Version != LedgerVersion {
		return Ledger{}, fail("idspace.ledger-version",
			"unsupported ledger version %d (expected %d)", ledger.Version, LedgerVersion)
	}
	if ledger.Next.IsZero() {
		next, scanErr := allocator.scanNext()
		if scanErr != nil {
			return Ledger{}, scanErr
		}
		ledger.Next = next
	}
	return ledger, nil
}

func (allocator *Allocator) saveLocked(ledger Ledger) error {
	ledger.Version = LedgerVersion
	data, err := json.MarshalIndent(ledger, "", "  ")
	if err != nil {
		return fail("idspace.write-failed", "%v", err)
	}
	return store.WriteAtomic(allocator.Layout.IDs(), append(data, '\n'))
}

// scanNext rebuilds the next ID from every registered book. It is the one-time initialization for
// a store that predates the ledger.
func (allocator *Allocator) scanNext() (model.ID, error) {
	registry, err := store.LoadRegistry(allocator.Layout.Registry())
	if err != nil {
		return model.ZeroID, fail("idspace.read-failed", "%v", err)
	}
	highest := model.ZeroID
	for _, entry := range registry.Books {
		manifest, manifestErr := allocator.readManifest(entry.ID)
		if manifestErr != nil {
			return model.ZeroID, manifestErr
		}
		if id := highestID(manifest); id > highest {
			highest = id
		}
	}
	if uint64(highest) == math.MaxUint64 {
		return model.ZeroID, fail("idspace.exhausted", "the highest existing ID leaves no successor")
	}
	if highest.IsZero() {
		return 1, nil
	}
	return highest + 1, nil
}

func (allocator *Allocator) readManifest(book model.BookID) (*model.Manifest, error) {
	paths, err := allocator.Layout.Book(book)
	if err != nil {
		return nil, fail("idspace.read-failed", "%v", err)
	}
	data, readErr := os.ReadFile(filepath.Join(paths.Dir, lifecycle.ManifestMember))
	if readErr == nil {
		manifest, decodeErr := model.Decode(data)
		if decodeErr != nil {
			return nil, fail("idspace.manifest-invalid", "book %q: %v", book, decodeErr)
		}
		return manifest, nil
	}
	if !errors.Is(readErr, os.ErrNotExist) {
		return nil, fail("idspace.read-failed", "%v", readErr)
	}
	container, loadErr := store.Load(paths.Container)
	if loadErr != nil {
		return nil, fail("idspace.book-missing", "book %q: %v", book, loadErr)
	}
	data, ok := container.Read(lifecycle.ManifestMember)
	if !ok {
		return nil, fail("idspace.manifest-missing",
			"book %q has no %s member", book, lifecycle.ManifestMember)
	}
	manifest, decodeErr := model.Decode(data)
	if decodeErr != nil {
		return nil, fail("idspace.manifest-invalid", "book %q: %v", book, decodeErr)
	}
	return manifest, nil
}

// highestID returns the highest ID the manifest owns. References are not owned IDs and are not
// considered.
func highestID(manifest *model.Manifest) model.ID {
	ids := manifest.OwnedIDs()
	if len(ids) == 0 {
		return model.ZeroID
	}
	return ids[len(ids)-1]
}

// PeekLedgerVersion reads the raw ledger version without validating or rebuilding it.
func PeekLedgerVersion(path string) (int, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, fail("idspace.ledger-missing", "%s does not exist", path)
	}
	if err != nil {
		return 0, fail("idspace.read-failed", "%v", err)
	}
	var header struct {
		Version int `json:"version"`
	}
	if decodeErr := json.Unmarshal(data, &header); decodeErr != nil {
		return 0, fail("idspace.ledger-invalid", "%v", decodeErr)
	}
	return header.Version, nil
}
