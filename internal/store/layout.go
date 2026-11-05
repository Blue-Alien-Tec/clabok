package store

import (
	"path/filepath"
	"strings"

	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

const (
	registryFile   = "library.json"
	idsFile        = "ids.json"
	termsFile      = "terms.txt"
	rollbackDir    = "rollback"
	containerExt   = ".book"
	blocksDir      = "blocks"
	layoutsDir     = "layouts"
	stateFile      = "state.json"
	openStateFile  = "open.json"
	coordinatorDir = ".coordinator"
)

// Layout is the fixed directory structure below one store root.
type Layout struct {
	Root string
}

// NewLayout resolves the store root. The root must be absolute; the effective configuration
// already guarantees that.
func NewLayout(root string) (Layout, error) {
	trimmed := strings.TrimSpace(root)
	if trimmed == "" {
		return Layout{}, fail("store.invalid-root", "the store root must not be empty")
	}
	if !filepath.IsAbs(trimmed) {
		return Layout{}, fail("store.invalid-root", "the store root must be absolute: %s", trimmed)
	}
	return Layout{Root: filepath.Clean(trimmed)}, nil
}

// Registry is the library registry file.
func (layout Layout) Registry() string { return filepath.Join(layout.Root, registryFile) }

// IDs is the library ID ledger file. The ledger is the library's authority for the global ID
// namespace: every issued ID comes from here, so uniqueness is a constraint, not a probability.
func (layout Layout) IDs() string { return filepath.Join(layout.Root, idsFile) }

// Terms is the term list file.
func (layout Layout) Terms() string { return filepath.Join(layout.Root, termsFile) }

// RollbackRoot holds the rollback packages, grouped by book.
func (layout Layout) RollbackRoot() string { return filepath.Join(layout.Root, rollbackDir) }

// Coordinator is the lock directory of the file backend.
func (layout Layout) Coordinator() string {
	return filepath.Join(layout.Root, coordinatorDir)
}

// OpenState is the shared open/close state file.
func (layout Layout) OpenState() string { return filepath.Join(layout.Root, openStateFile) }

// BookPaths are the paths of one book. The book ID is the identity; the output path is not part of
// the store.
type BookPaths struct {
	ID          model.BookID
	Dir         string
	Container   string
	BlocksDir   string
	LayoutsDir  string
	RollbackDir string
	StateFile   string
}

// Book resolves the paths of one book and rejects an invalid ID.
func (layout Layout) Book(id model.BookID) (BookPaths, error) {
	if !id.Valid() {
		return BookPaths{}, fail("store.invalid-book-id", "invalid book id %q", string(id))
	}
	directory := filepath.Join(layout.Root, string(id))
	return BookPaths{
		ID:          id,
		Dir:         directory,
		Container:   filepath.Join(directory, string(id)+containerExt),
		BlocksDir:   filepath.Join(directory, blocksDir),
		LayoutsDir:  filepath.Join(directory, layoutsDir),
		RollbackDir: filepath.Join(layout.RollbackRoot(), string(id)),
		StateFile:   filepath.Join(directory, stateFile),
	}, nil
}
