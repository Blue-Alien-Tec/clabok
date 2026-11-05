// Package rollback stores undo packages and book-level rollback points.
//
// A package is a verified member set (the same container codec as a book) stored under
// `<store>/rollback/<bookId>/`. Undo packages are short-lived and capped per book; book-level points
// survive a close and are capped separately.
package rollback

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

// Kind classifies a rollback package.
type Kind string

const (
	// KindUndo is a short-lived package created before a mutating command.
	KindUndo Kind = "undo"
	// KindBook is a book-level point created at close when the book changed.
	KindBook Kind = "book"
	// KindBody is a body-scoped undo package created before one body write.
	KindBody Kind = "body"
)

const (
	timeLayout = "20060102T150405.000000000Z"
	extension  = ".zip"
)

// Package describes one stored package.
type Package struct {
	Name    string
	Kind    Kind
	Reason  string
	Created time.Time
	Size    int64
}

// Store is the rollback pool of one book.
type Store struct {
	dir       string
	undoLimit int
	bookLimit int
	now       func() time.Time
	last      time.Time
}

// Error is a rollback failure with a stable code.
type Error struct {
	Code    string
	Message string
}

// Error returns the human message.
func (err *Error) Error() string { return err.Message }

// ErrorCode returns the stable code for the CLI boundary.
func (err *Error) ErrorCode() string { return err.Code }

func fail(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// New returns the rollback pool for a directory. A limit of zero disables that kind.
func New(directory string, undoLimit, bookLimit int) (*Store, error) {
	if directory == "" || !filepath.IsAbs(directory) {
		return nil, fail("rollback.invalid-root", "the rollback directory must be an absolute path")
	}
	if undoLimit < 0 || bookLimit < 0 {
		return nil, fail("rollback.invalid-limit", "rollback limits must not be negative")
	}
	return &Store{
		dir:       filepath.Clean(directory),
		undoLimit: undoLimit,
		bookLimit: bookLimit,
		now:       time.Now,
	}, nil
}

// Snapshot stores a member set and prunes the pool of its kind.
func (store_ *Store) Snapshot(kind Kind, members map[string][]byte, reason string) (Package, error) {
	limit := store_.limit(kind)
	if limit == 0 {
		return Package{}, nil
	}
	if kind != KindUndo && kind != KindBook && kind != KindBody {
		return Package{}, fail("rollback.invalid-kind", "unknown package kind %q", kind)
	}
	if err := os.MkdirAll(store_.dir, 0o755); err != nil {
		return Package{}, fail("rollback.write-failed", "%v", err)
	}
	created := store_.nextTimestamp()
	cleanReason := sanitizeReason(reason)
	name := fmt.Sprintf("%s-%s-%s-%s%s", kind, created.Format(timeLayout), uniqueSuffix(), cleanReason, extension)

	container := store.New()
	for member, data := range members {
		if err := container.Write(member, data); err != nil {
			return Package{}, err
		}
	}
	if err := container.Save(filepath.Join(store_.dir, name)); err != nil {
		return Package{}, err
	}
	if err := store_.prune(kind); err != nil {
		return Package{}, err
	}
	return Package{Name: name, Kind: kind, Reason: cleanReason, Created: created}, nil
}

// List returns all packages, oldest first.
func (store_ *Store) List() ([]Package, error) {
	entries, err := os.ReadDir(store_.dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fail("rollback.read-failed", "%v", err)
	}
	packages := make([]Package, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		pkg, ok := parseName(entry.Name())
		if !ok {
			continue
		}
		if info, infoErr := entry.Info(); infoErr == nil {
			pkg.Size = info.Size()
		}
		packages = append(packages, pkg)
	}
	sort.Slice(packages, func(left, right int) bool {
		return packages[left].Name < packages[right].Name
	})
	return packages, nil
}

// Load returns a package and its member set.
func (store_ *Store) Load(name string) (Package, map[string][]byte, error) {
	if filepath.Base(name) != name {
		return Package{}, nil, fail("rollback.invalid-package", "invalid package name %q", name)
	}
	pkg, ok := parseName(name)
	if !ok {
		return Package{}, nil, fail("rollback.invalid-package", "unknown package %q", name)
	}
	container, err := store.Load(filepath.Join(store_.dir, name))
	if err != nil {
		return Package{}, nil, err
	}
	members := make(map[string][]byte, container.Len())
	for _, member := range container.Names() {
		data, _ := container.Read(member)
		members[member] = data
	}
	return pkg, members, nil
}

// Clear removes every package of one kind.
func (store_ *Store) Clear(kind Kind) error {
	packages, err := store_.List()
	if err != nil {
		return err
	}
	for _, pkg := range packages {
		if pkg.Kind != kind {
			continue
		}
		if err := os.Remove(filepath.Join(store_.dir, pkg.Name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fail("rollback.write-failed", "%v", err)
		}
	}
	return nil
}

func (store_ *Store) limit(kind Kind) int {
	if kind == KindBook {
		return store_.bookLimit
	}
	return store_.undoLimit
}

// nextTimestamp keeps names strictly increasing even when the clock has coarse resolution.
func (store_ *Store) nextTimestamp() time.Time {
	current := store_.now().UTC()
	if !current.After(store_.last) {
		current = store_.last.Add(time.Nanosecond)
	}
	store_.last = current
	return current
}

func (store_ *Store) prune(kind Kind) error {
	limit := store_.limit(kind)
	if limit <= 0 {
		return nil
	}
	packages, err := store_.List()
	if err != nil {
		return err
	}
	matching := make([]Package, 0, len(packages))
	for _, pkg := range packages {
		if pkg.Kind == kind {
			matching = append(matching, pkg)
		}
	}
	if len(matching) <= limit {
		return nil
	}
	for _, pkg := range matching[:len(matching)-limit] {
		if err := os.Remove(filepath.Join(store_.dir, pkg.Name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fail("rollback.write-failed", "%v", err)
		}
	}
	return nil
}

func parseName(name string) (Package, bool) {
	if !strings.HasSuffix(name, extension) {
		return Package{}, false
	}
	parts := strings.SplitN(strings.TrimSuffix(name, extension), "-", 4)
	if len(parts) != 4 {
		return Package{}, false
	}
	kind := Kind(parts[0])
	if kind != KindUndo && kind != KindBook && kind != KindBody {
		return Package{}, false
	}
	created, err := time.Parse(timeLayout, parts[1])
	if err != nil {
		return Package{}, false
	}
	return Package{Name: name, Kind: kind, Reason: parts[3], Created: created}, true
}

// uniqueSuffix keeps package names distinct when several writers snapshot concurrently.
func uniqueSuffix() string {
	var buffer [3]byte
	if _, err := rand.Read(buffer[:]); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano()&0xffffff)
	}
	return hex.EncodeToString(buffer[:])
}

func sanitizeReason(reason string) string {
	var builder strings.Builder
	for _, character := range strings.ToLower(reason) {
		alphanumeric := (character >= 'a' && character <= 'z') ||
			(character >= '0' && character <= '9')
		if alphanumeric || character == '.' || character == '_' {
			builder.WriteRune(character)
		} else {
			builder.WriteByte('-')
		}
	}
	text := strings.Trim(builder.String(), "-.")
	if text == "" {
		return "change"
	}
	if len(text) > 40 {
		text = text[:40]
	}
	return text
}
