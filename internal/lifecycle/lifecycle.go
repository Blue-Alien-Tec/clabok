// Package lifecycle owns the open/close state of a book.
//
// A closed book is the packed container. Opening extracts the manifest and the bodies into the book
// directory; the extracted files are authoritative until close repacks them. The open state is
// global — shared by all processes — and tracked in one state file, not inferred from folder
// contents.
package lifecycle

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Blue-Alien-Tec/clabok/internal/coordinator"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/rollback"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

// ManifestMember is the manifest member name inside the container and the extracted file name.
const ManifestMember = "manifest.json"

// Error is a lifecycle failure with a stable code.
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

// Status is the open state of one book.
type Status string

const (
	// StatusClosed means the container is authoritative and nothing is extracted.
	StatusClosed Status = "closed"
	// StatusOpen means the extracted files and an open entry exist.
	StatusOpen Status = "open"
	// StatusUnsaved means extracted files exist without an open entry: an interrupted open or close.
	StatusUnsaved Status = "unsaved"
	// StatusOrphaned means an open entry exists without extracted files.
	StatusOrphaned Status = "orphaned"
)

// Entry is the open-state record of one book.
type Entry struct {
	ID         model.BookID `json:"id"`
	OutputPath string       `json:"outputPath,omitempty"`
	PID        int          `json:"pid"`
	Session    string       `json:"session"`
	OpenedAt   string       `json:"openedAt"`
}

// State is the observed state of one book.
type State struct {
	ID        model.BookID
	Status    Status
	Extracted bool
	Entry     Entry
	HasEntry  bool
}

// OpenResult reports a successful open.
type OpenResult struct {
	Paths     store.BookPaths
	Entry     Entry
	Recovered bool
}

// CloseResult reports a successful close.
type CloseResult struct {
	Paths        store.BookPaths
	Repacked     bool
	EntryRemoved bool
	Recovered    bool
	BookPoint    bool
}

// RestoreResult reports a successful restore.
type RestoreResult struct {
	Package rollback.Package
	Members int
}

// Options controls the rollback pools. A limit of zero disables that kind.
type Options struct {
	UndoLimit      int
	BookPointLimit int
}

// Lifecycle coordinates the open/close state of all books in one store.
type Lifecycle struct {
	layout      store.Layout
	coordinator coordinator.Coordinator
	options     Options
	now         func() time.Time
	pid         int
}

// New returns a lifecycle over one store layout and coordinator.
func New(layout store.Layout, coordinatorBackend coordinator.Coordinator, options Options) *Lifecycle {
	return &Lifecycle{
		layout:      layout,
		coordinator: coordinatorBackend,
		options:     options,
		now:         time.Now,
		pid:         os.Getpid(),
	}
}

// Open extracts a book and records the open state. It is idempotent and repairs the two crash
// states in place, reporting the repair in the result.
func (life *Lifecycle) Open(id model.BookID, outputPath string) (OpenResult, error) {
	paths, err := life.layout.Book(id)
	if err != nil {
		return OpenResult{}, err
	}
	release, err := life.acquire(id)
	if err != nil {
		return OpenResult{}, err
	}
	defer func() { _ = release() }()

	file, err := life.readOpenFile()
	if err != nil {
		return OpenResult{}, err
	}
	entry, hasEntry := file.find(id)
	if extracted(paths) {
		if hasEntry {
			return OpenResult{Paths: paths, Entry: entry}, nil
		}
		entry = life.newEntry(id, outputPath)
		file.upsert(entry)
		if err := life.writeOpenFile(file); err != nil {
			return OpenResult{}, err
		}
		return OpenResult{Paths: paths, Entry: entry, Recovered: true}, nil
	}

	if err := life.extract(paths, id); err != nil {
		return OpenResult{}, err
	}
	entry = life.newEntry(id, outputPath)
	file.upsert(entry)
	if err := life.writeOpenFile(file); err != nil {
		return OpenResult{}, err
	}
	return OpenResult{Paths: paths, Entry: entry, Recovered: hasEntry}, nil
}

// Close repacks the extracted state into the container and removes the open entry. It is idempotent
// and repairs the two crash states in place.
func (life *Lifecycle) Close(id model.BookID) (CloseResult, error) {
	paths, err := life.layout.Book(id)
	if err != nil {
		return CloseResult{}, err
	}
	release, err := life.acquire(id)
	if err != nil {
		return CloseResult{}, err
	}
	defer func() { _ = release() }()

	file, err := life.readOpenFile()
	if err != nil {
		return CloseResult{}, err
	}
	_, hasEntry := file.find(id)
	if !extracted(paths) {
		if hasEntry {
			file.remove(id)
			if err := life.writeOpenFile(file); err != nil {
				return CloseResult{}, err
			}
			return CloseResult{Paths: paths, EntryRemoved: true, Recovered: true}, nil
		}
		return CloseResult{Paths: paths}, nil
	}

	container, err := life.buildContainer(paths)
	if err != nil {
		return CloseResult{}, err
	}
	newData, err := container.Marshal()
	if err != nil {
		return CloseResult{}, err
	}
	bookPoint, err := life.recordBookPoint(paths, newData)
	if err != nil {
		return CloseResult{}, err
	}
	if err := container.Save(paths.Container); err != nil {
		return CloseResult{}, err
	}
	if err := os.RemoveAll(paths.BlocksDir); err != nil {
		return CloseResult{}, fail("lifecycle.write-failed", "%v", err)
	}
	if err := os.RemoveAll(paths.LayoutsDir); err != nil {
		return CloseResult{}, fail("lifecycle.write-failed", "%v", err)
	}
	if err := os.Remove(filepath.Join(paths.Dir, ManifestMember)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return CloseResult{}, fail("lifecycle.write-failed", "%v", err)
	}
	removed := false
	if hasEntry {
		file.remove(id)
		if err := life.writeOpenFile(file); err != nil {
			return CloseResult{}, err
		}
		removed = true
	}
	if rollbacks, rollbackErr := life.rollbackStore(paths); rollbackErr == nil {
		_ = rollbacks.Clear(rollback.KindUndo)
		_ = rollbacks.Clear(rollback.KindBody)
	}
	return CloseResult{
		Paths:        paths,
		Repacked:     true,
		EntryRemoved: removed,
		Recovered:    !hasEntry,
		BookPoint:    bookPoint,
	}, nil
}

// State reports the observed state without locking; the state file is read atomically.
func (life *Lifecycle) State(id model.BookID) (State, error) {
	paths, err := life.layout.Book(id)
	if err != nil {
		return State{}, err
	}
	file, err := life.readOpenFile()
	if err != nil {
		return State{}, err
	}
	entry, hasEntry := file.find(id)
	isExtracted := extracted(paths)
	status := StatusClosed
	switch {
	case isExtracted && hasEntry:
		status = StatusOpen
	case isExtracted && !hasEntry:
		status = StatusUnsaved
	case !isExtracted && hasEntry:
		status = StatusOrphaned
	}
	return State{ID: id, Status: status, Extracted: isExtracted, Entry: entry, HasEntry: hasEntry}, nil
}

// Paths returns the store paths of one book without inspecting or changing its open state.
func (life *Lifecycle) Paths(id model.BookID) (store.BookPaths, error) {
	return life.layout.Book(id)
}

// RequireOpen returns the book paths or the lifecycle.not-open error.
func (life *Lifecycle) RequireOpen(id model.BookID) (store.BookPaths, error) {
	state, err := life.State(id)
	if err != nil {
		return store.BookPaths{}, err
	}
	if !state.Extracted {
		return store.BookPaths{}, fail("lifecycle.not-open",
			"book %q is not open; run `pecia book open %s`", id, id)
	}
	return life.layout.Book(id)
}

// RequireClosed returns the lifecycle.open error while the book is open or has an open entry.
func (life *Lifecycle) RequireClosed(id model.BookID) error {
	state, err := life.State(id)
	if err != nil {
		return err
	}
	if state.HasEntry || state.Extracted {
		return fail("lifecycle.open", "book %q is open; close it first", id)
	}
	return nil
}

func (life *Lifecycle) acquire(id model.BookID) (coordinator.Release, error) {
	return life.coordinator.Acquire([]coordinator.Resource{
		{Kind: coordinator.KindStore},
		{Kind: coordinator.KindBook, Book: id},
	}, coordinator.Exclusive, 0)
}

// LockStore holds the store-level exclusive lock. Registry mutations use it so two processes cannot
// interleave a read-modify-write of the library registry.
func (life *Lifecycle) LockStore() (coordinator.Release, error) {
	return life.coordinator.Acquire([]coordinator.Resource{
		{Kind: coordinator.KindStore},
	}, coordinator.Exclusive, 0)
}

// LockIDs holds the library ID ledger exclusively. The ledger holder never waits for a store or
// book lock, so it may be acquired while a book lock is held without a deadlock.
func (life *Lifecycle) LockIDs() (coordinator.Release, error) {
	return life.coordinator.Acquire([]coordinator.Resource{
		{Kind: coordinator.KindIDs},
	}, coordinator.Exclusive, 0)
}

// LockBookExclusive holds the book exclusively for a manifest mutation. The caller does not touch
// the open state, so the store lock is not needed.
func (life *Lifecycle) LockBookExclusive(id model.BookID) (coordinator.Release, error) {
	return life.coordinator.Acquire([]coordinator.Resource{
		{Kind: coordinator.KindBook, Book: id},
	}, coordinator.Exclusive, 0)
}

// LockBookShared holds the book shared while a body write runs. A close waits for it.
func (life *Lifecycle) LockBookShared(id model.BookID) (coordinator.Release, error) {
	return life.coordinator.Acquire([]coordinator.Resource{
		{Kind: coordinator.KindBook, Book: id},
	}, coordinator.Shared, 0)
}

// LockBlock holds one block exclusively. Call it after LockBookShared: the order is book -> block.
func (life *Lifecycle) LockBlock(book model.BookID, block model.ID) (coordinator.Release, error) {
	return life.coordinator.Acquire([]coordinator.Resource{
		{Kind: coordinator.KindBlock, Book: book, ID: block},
	}, coordinator.Exclusive, 0)
}

func (life *Lifecycle) extract(paths store.BookPaths, id model.BookID) error {
	if _, err := os.Stat(paths.Container); errors.Is(err, os.ErrNotExist) {
		return fail("lifecycle.no-container", "no container for book %q at %s", id, paths.Container)
	} else if err != nil {
		return fail("lifecycle.read-failed", "%v", err)
	}
	container, err := store.Load(paths.Container)
	if err != nil {
		return err
	}
	if _, err := container.Export(paths.Dir); err != nil {
		return err
	}
	return nil
}

// buildContainer rebuilds the container from the extracted state and validates it before anything
// is written.
func (life *Lifecycle) buildContainer(paths store.BookPaths) (*store.Container, error) {
	manifestData, err := os.ReadFile(filepath.Join(paths.Dir, ManifestMember))
	if errors.Is(err, os.ErrNotExist) {
		return nil, fail("lifecycle.extracted-incomplete",
			"%s is missing; the book cannot be packed", ManifestMember)
	}
	if err != nil {
		return nil, fail("lifecycle.read-failed", "%v", err)
	}
	manifest, err := model.Decode(manifestData)
	if err != nil {
		return nil, err
	}
	book := model.NewBook(manifest)
	for _, id := range manifest.PlacedBlocks() {
		book.SetBody(id, "")
	}
	entries, readErr := os.ReadDir(paths.BlocksDir)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return nil, fail("lifecycle.read-failed", "%v", readErr)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		id, parseErr := model.ParseID(strings.TrimSuffix(entry.Name(), ".md"))
		if parseErr != nil {
			continue
		}
		data, fileErr := os.ReadFile(filepath.Join(paths.BlocksDir, entry.Name()))
		if fileErr != nil {
			return nil, fail("lifecycle.read-failed", "%v", fileErr)
		}
		book.SetBody(id, string(data))
	}
	if validation := book.Validate(); validation != nil {
		return nil, validation
	}
	encoded, err := manifest.Encode()
	if err != nil {
		return nil, fail("lifecycle.encode-failed", "%v", err)
	}
	container := store.New()
	if err := container.Write(ManifestMember, encoded); err != nil {
		return nil, err
	}
	for id, body := range book.Bodies {
		if err := container.Write("blocks/"+id.String()+".md", []byte(body)); err != nil {
			return nil, err
		}
	}
	layouts, layoutErr := collectLayouts(paths)
	if layoutErr != nil {
		return nil, layoutErr
	}
	for name, data := range layouts {
		if err := container.Write(name, data); err != nil {
			return nil, err
		}
	}
	return container, nil
}

// Snapshot records an undo package of the extracted state. The caller must hold the book lock.
func (life *Lifecycle) Snapshot(id model.BookID, reason string) (rollback.Package, error) {
	paths, err := life.layout.Book(id)
	if err != nil {
		return rollback.Package{}, err
	}
	if !extracted(paths) {
		return rollback.Package{}, fail("lifecycle.not-open", "book %q is not open", id)
	}
	members, err := CollectMembers(paths)
	if err != nil {
		return rollback.Package{}, err
	}
	rollbacks, err := life.rollbackStore(paths)
	if err != nil {
		return rollback.Package{}, err
	}
	return rollbacks.Snapshot(rollback.KindUndo, members, reason)
}

// SnapshotBody records a body-scoped undo package. It reads one body, so parallel writes on other
// blocks are neither copied nor blocked.
func (life *Lifecycle) SnapshotBody(id model.BookID, block model.ID, reason string) (rollback.Package, error) {
	paths, err := life.layout.Book(id)
	if err != nil {
		return rollback.Package{}, err
	}
	if !extracted(paths) {
		return rollback.Package{}, fail("lifecycle.not-open", "book %q is not open", id)
	}
	var data []byte
	fileData, readErr := os.ReadFile(filepath.Join(paths.BlocksDir, block.String()+".md"))
	if readErr == nil {
		data = fileData
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return rollback.Package{}, fail("lifecycle.read-failed", "%v", readErr)
	}
	rollbacks, err := life.rollbackStore(paths)
	if err != nil {
		return rollback.Package{}, err
	}
	return rollbacks.Snapshot(rollback.KindBody, map[string][]byte{
		"blocks/" + block.String() + ".md": data,
	}, reason)
}

// Restore applies a rollback package. An undo package restores the extracted state; a book-level
// point restores the container. The state before the restore is snapshotted first.
func (life *Lifecycle) Restore(id model.BookID, name string) (RestoreResult, error) {
	paths, err := life.layout.Book(id)
	if err != nil {
		return RestoreResult{}, err
	}
	release, err := life.acquire(id)
	if err != nil {
		return RestoreResult{}, err
	}
	defer func() { _ = release() }()

	rollbacks, err := life.rollbackStore(paths)
	if err != nil {
		return RestoreResult{}, err
	}
	pkg, members, err := rollbacks.Load(name)
	if err != nil {
		return RestoreResult{}, err
	}
	switch pkg.Kind {
	case rollback.KindUndo:
		if !extracted(paths) {
			return RestoreResult{}, fail("lifecycle.not-open",
				"book %q is not open; run `pecia book open %s`", id, id)
		}
		if _, exists := members[ManifestMember]; !exists {
			return RestoreResult{}, fail("rollback.invalid-package",
				"an undo package needs %s", ManifestMember)
		}
		current, collectErr := CollectMembers(paths)
		if collectErr != nil {
			return RestoreResult{}, collectErr
		}
		if _, snapErr := rollbacks.Snapshot(rollback.KindUndo, current, "restore"); snapErr != nil {
			return RestoreResult{}, snapErr
		}
		if replaceErr := replaceExtracted(paths, members); replaceErr != nil {
			return RestoreResult{}, replaceErr
		}
	case rollback.KindBody:
		if !extracted(paths) {
			return RestoreResult{}, fail("lifecycle.not-open",
				"book %q is not open; run `pecia book open %s`", id, id)
		}
		blockID, data, memberErr := singleBlockMember(members)
		if memberErr != nil {
			return RestoreResult{}, memberErr
		}
		if _, snapErr := life.SnapshotBody(id, blockID, "restore"); snapErr != nil {
			return RestoreResult{}, snapErr
		}
		if writeErr := store.WriteAtomic(filepath.Join(paths.BlocksDir, blockID.String()+".md"), data); writeErr != nil {
			return RestoreResult{}, writeErr
		}
	case rollback.KindBook:
		if extracted(paths) {
			return RestoreResult{}, fail("lifecycle.open", "book %q is open; close it first", id)
		}
		if oldData, readErr := os.ReadFile(paths.Container); readErr == nil {
			if old, parseErr := store.Parse(oldData); parseErr == nil {
				if _, snapErr := rollbacks.Snapshot(rollback.KindBook, membersOf(old), "restore"); snapErr != nil {
					return RestoreResult{}, snapErr
				}
			}
		}
		container := store.New()
		for member, data := range members {
			if writeErr := container.Write(member, data); writeErr != nil {
				return RestoreResult{}, writeErr
			}
		}
		if saveErr := container.Save(paths.Container); saveErr != nil {
			return RestoreResult{}, saveErr
		}
	default:
		return RestoreResult{}, fail("rollback.invalid-kind", "unknown package kind %q", pkg.Kind)
	}
	return RestoreResult{Package: pkg, Members: len(members)}, nil
}

// RollbackStore exposes the rollback pool of a book for listing and inspection.
func (life *Lifecycle) RollbackStore(id model.BookID) (*rollback.Store, error) {
	paths, err := life.layout.Book(id)
	if err != nil {
		return nil, err
	}
	return life.rollbackStore(paths)
}

func (life *Lifecycle) rollbackStore(paths store.BookPaths) (*rollback.Store, error) {
	return rollback.New(paths.RollbackDir, life.options.UndoLimit, life.options.BookPointLimit)
}

func (life *Lifecycle) recordBookPoint(paths store.BookPaths, newData []byte) (bool, error) {
	oldData, err := os.ReadFile(paths.Container)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fail("lifecycle.read-failed", "%v", err)
	}
	if bytes.Equal(oldData, newData) {
		return false, nil
	}
	old, parseErr := store.Parse(oldData)
	if parseErr != nil {
		return false, parseErr
	}
	rollbacks, rbErr := life.rollbackStore(paths)
	if rbErr != nil {
		return false, rbErr
	}
	pkg, snapErr := rollbacks.Snapshot(rollback.KindBook, membersOf(old), "close")
	if snapErr != nil {
		return false, snapErr
	}
	return pkg.Name != "", nil
}

// CollectMembers returns the extracted manifest and bodies as a member set.
func CollectMembers(paths store.BookPaths) (map[string][]byte, error) {
	manifestData, err := os.ReadFile(filepath.Join(paths.Dir, ManifestMember))
	if errors.Is(err, os.ErrNotExist) {
		return nil, fail("lifecycle.extracted-incomplete", "%s is missing", ManifestMember)
	}
	if err != nil {
		return nil, fail("lifecycle.read-failed", "%v", err)
	}
	members := map[string][]byte{ManifestMember: manifestData}
	entries, readErr := os.ReadDir(paths.BlocksDir)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return nil, fail("lifecycle.read-failed", "%v", readErr)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if _, parseErr := model.ParseID(strings.TrimSuffix(entry.Name(), ".md")); parseErr != nil {
			continue
		}
		data, fileErr := os.ReadFile(filepath.Join(paths.BlocksDir, entry.Name()))
		if fileErr != nil {
			return nil, fail("lifecycle.read-failed", "%v", fileErr)
		}
		members["blocks/"+entry.Name()] = data
	}
	layouts, layErr := collectLayouts(paths)
	if layErr != nil {
		return nil, layErr
	}
	for name, data := range layouts {
		members[name] = data
	}
	return members, nil
}

// collectLayouts reads the extracted ASCII UI specs as container members under
// layouts/asciiui/<blockID>.asciiui.
func collectLayouts(paths store.BookPaths) (map[string][]byte, error) {
	members := map[string][]byte{}
	directory := filepath.Join(paths.LayoutsDir, "asciiui")
	entries, err := os.ReadDir(directory)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return members, nil
		}
		return nil, fail("lifecycle.read-failed", "%v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".asciiui") {
			continue
		}
		id, parseErr := model.ParseID(strings.TrimSuffix(entry.Name(), ".asciiui"))
		if parseErr != nil || id.IsZero() {
			continue
		}
		data, fileErr := os.ReadFile(filepath.Join(directory, entry.Name()))
		if fileErr != nil {
			return nil, fail("lifecycle.read-failed", "%v", fileErr)
		}
		members["layouts/asciiui/"+entry.Name()] = data
	}
	return members, nil
}

func replaceExtracted(paths store.BookPaths, members map[string][]byte) error {
	if err := os.RemoveAll(paths.BlocksDir); err != nil {
		return fail("lifecycle.write-failed", "%v", err)
	}
	if err := os.RemoveAll(paths.LayoutsDir); err != nil {
		return fail("lifecycle.write-failed", "%v", err)
	}
	if err := os.Remove(filepath.Join(paths.Dir, ManifestMember)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fail("lifecycle.write-failed", "%v", err)
	}
	for member, data := range members {
		target := filepath.Join(paths.Dir, filepath.FromSlash(member))
		if err := store.WriteAtomic(target, data); err != nil {
			return err
		}
	}
	return nil
}

func membersOf(container *store.Container) map[string][]byte {
	members := make(map[string][]byte, container.Len())
	for _, name := range container.Names() {
		data, _ := container.Read(name)
		members[name] = data
	}
	return members
}

// singleBlockMember extracts the one body member of a body-scoped package.
func singleBlockMember(members map[string][]byte) (model.ID, []byte, error) {
	if len(members) != 1 {
		return model.ZeroID, nil, fail("rollback.invalid-package",
			"a body package holds exactly one body, got %d", len(members))
	}
	for name, data := range members {
		if !strings.HasPrefix(name, "blocks/") || !strings.HasSuffix(name, ".md") {
			return model.ZeroID, nil, fail("rollback.invalid-package",
				"a body package holds only blocks/<id>.md, got %q", name)
		}
		id, err := model.ParseID(strings.TrimSuffix(strings.TrimPrefix(name, "blocks/"), ".md"))
		if err != nil || id.IsZero() {
			return model.ZeroID, nil, fail("rollback.invalid-package", "invalid body member %q", name)
		}
		return id, data, nil
	}
	return model.ZeroID, nil, fail("rollback.invalid-package", "empty body package")
}

func (life *Lifecycle) newEntry(id model.BookID, outputPath string) Entry {
	return Entry{
		ID:         id,
		OutputPath: outputPath,
		PID:        life.pid,
		Session:    newSession(),
		OpenedAt:   life.now().UTC().Format(time.RFC3339),
	}
}

func newSession() string {
	var buffer [8]byte
	if _, err := rand.Read(buffer[:]); err != nil {
		return fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UnixNano())
	}
	return hex.EncodeToString(buffer[:])
}

func extracted(paths store.BookPaths) bool {
	info, err := os.Stat(filepath.Join(paths.Dir, ManifestMember))
	return err == nil && !info.IsDir()
}
