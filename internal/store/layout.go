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
	pendingTerms   = "pending-terms.json"
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

// TermsPending is the pending term-request store.
func (layout Layout) TermsPending() string { return filepath.Join(layout.Root, pendingTerms) }

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

// ValidateBookID rejects ids that cannot safely name a store folder. The check is case-insensitive
// for reserved names and rejects trailing dots and Windows-reserved characters, because a
// case-insensitive file system would otherwise resolve an alias to store infrastructure.
func ValidateBookID(id model.BookID) error {
	if !id.Valid() {
		return fail("store.invalid-book-id", "invalid book id %q", string(id))
	}
	if AmbiguousBookID(id) {
		return fail("store.reserved-book-id",
			"book id %q contains a reserved character or a trailing dot or space", string(id))
	}
	if ReservedBookID(id) {
		return fail("store.reserved-book-id", "book id %q is part of the store layout", string(id))
	}
	return nil
}

// ReservedBookID reports ids that name store infrastructure rather than a book directory. The
// comparison is case-insensitive and ignores trailing dots, because a case-insensitive file system
// resolves "ROLLBACK" and "rollback." to the same directory as "rollback".
func ReservedBookID(id model.BookID) bool {
	name := strings.TrimRight(string(id), ". ")
	switch strings.ToLower(name) {
	case "rollback", ".coordinator", "library.json", "ids.json", "terms.txt",
		"pending-terms.json", "open.json", "index", "store.json", "migrations":
		return true
	}
	return false
}

// AmbiguousBookID reports ids that are not a plain path component on every platform. A trailing dot
// is stripped by Windows, and a colon can address an alternate data stream; both would let an id
// alias a different path than the caller intended.
func AmbiguousBookID(id model.BookID) bool {
	name := string(id)
	if strings.TrimRight(name, ". ") != name {
		return true
	}
	return strings.ContainsAny(name, `:*?"<>|`)
}

// BookIDKey is the case-insensitive identity used for duplicate checks. On a case-insensitive file
// system two ids that differ only by case name the same store directory.
func BookIDKey(id model.BookID) string { return strings.ToLower(string(id)) }

// PathKey is the case-insensitive identity used for output-path duplicate checks. Trailing dots
// and spaces are removed per segment because Windows normalizes them away.
func PathKey(path string) string {
	clean := filepath.ToSlash(filepath.Clean(path))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return strings.ToLower(clean)
	}
	segments := strings.Split(clean, "/")
	for index, segment := range segments {
		segments[index] = strings.TrimRight(segment, ". ")
	}
	return strings.ToLower(strings.Join(segments, "/"))
}

// Book resolves the paths of one book and rejects an invalid or reserved ID.
func (layout Layout) Book(id model.BookID) (BookPaths, error) {
	if err := ValidateBookID(id); err != nil {
		return BookPaths{}, err
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
