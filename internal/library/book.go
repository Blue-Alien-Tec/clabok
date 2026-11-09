package library

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/render"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

// NewBookOptions is one `book new` request. Pointer fields distinguish "not given" from a zero
// value, so an explicit false or 0 still wins over the library default.
type NewBookOptions struct {
	Title        string
	ID           string
	Path         string
	AllowOutside bool
	Library      bool
	Force        bool
	Numbering    *bool
	TOC          *bool
	IndexDepth   *int
	Output       string
}

// NewBookResult is the receipt of `book new`.
type NewBookResult struct {
	DryRun   bool           `json:"dryRun"`
	ID       string         `json:"id"`
	Path     string         `json:"path"`
	Kind     string         `json:"kind"`
	Root     string         `json:"root,omitempty"`
	Settings model.Settings `json:"settings"`
	Files    []string       `json:"files,omitempty"`
	Written  bool           `json:"written"`
}

// DryRunResult marks the payload as a dry-run receipt.
func (result NewBookResult) DryRunResult() bool { return result.DryRun }

func (result NewBookResult) Text() string {
	if result.DryRun {
		return fmt.Sprintf("dry run: create %s (%s) at %s", result.ID, result.Kind, result.Path)
	}
	return fmt.Sprintf("%s: created %s at %s (%d file(s))", result.ID, result.Kind, result.Path, len(result.Files))
}

// NewBook creates a closed container, registers it and renders its first output.
func (service *Service) NewBook(options NewBookOptions, dryRun bool) (NewBookResult, *cli.Error) {
	title := model.NormalizeTitle(options.Title)
	if strings.TrimSpace(title) == "" {
		return NewBookResult{}, cli.Usage("usage.missing-argument", "--title is required")
	}
	if options.Library && (options.Path != "" || options.AllowOutside) {
		return NewBookResult{}, cli.Usage("usage.invalid-arguments",
			"a library book has the fixed output path \".\"; --path and --allow-outside do not apply")
	}
	release, lockErr := service.Life.LockStore()
	if lockErr != nil {
		return NewBookResult{}, cli.AsError(lockErr)
	}
	defer func() { _ = release() }()

	registry, registryErr := service.loadRegistry()
	if registryErr != nil {
		return NewBookResult{}, registryErr
	}
	kind := store.KindBook
	if options.Library {
		if _, exists := registry.Library(); exists {
			return NewBookResult{}, cli.Reject("library.library-exists",
				"the library already has a book").
				WithHint("rename or unregister the existing library entry first")
		}
		kind = store.KindLibrary
	}
	id, idErr := service.newBookID(registry, options.ID, title)
	if idErr != nil {
		return NewBookResult{}, idErr
	}
	outputPath, pathErr := service.newBookPath(registry, options, id)
	if pathErr != nil {
		return NewBookResult{}, pathErr
	}
	settings, settingsErr := effectiveSettings(registry, options)
	if settingsErr != nil {
		return NewBookResult{}, settingsErr
	}
	result := NewBookResult{
		DryRun: dryRun, ID: string(id), Path: outputPath, Kind: kind, Settings: settings,
	}
	if dryRun {
		return result, nil
	}
	if service.IDs == nil {
		return NewBookResult{}, cli.Fail("library.allocator-missing",
			"the library ID allocator is not configured")
	}
	manifest, manifestErr := model.NewManifest(id, title, service.IDs)
	if manifestErr != nil {
		return NewBookResult{}, cli.AsError(manifestErr)
	}
	manifest.Settings = settings
	result.Root = manifest.Root.ID.String()
	if kind == store.KindBook {
		target := service.target(outputPath)
		empty, emptyErr := dirIsEmpty(target)
		if emptyErr != nil {
			return NewBookResult{}, cli.Fail("library.path-read-failed", "%v", emptyErr)
		}
		if !empty && !options.Force {
			return NewBookResult{}, cli.Reject("library.path-exists",
				"%s already exists and is not empty", target).
				WithHint("pass --force to render into the existing directory")
		}
	}
	paths, pathsErr := service.Layout.Book(id)
	if pathsErr != nil {
		return NewBookResult{}, cli.AsError(pathsErr)
	}
	occupied, occupiedErr := dirExists(paths.Dir)
	if occupiedErr != nil {
		return NewBookResult{}, cli.Fail("library.store-read-failed", "%v", occupiedErr)
	}
	if occupied {
		empty, emptyErr := dirIsEmpty(paths.Dir)
		if emptyErr != nil {
			return NewBookResult{}, cli.Fail("library.store-read-failed", "%v", emptyErr)
		}
		if !empty {
			return NewBookResult{}, cli.Reject("library.store-exists",
				"the store directory for book %q is not empty", id).
				WithHint("run `pecia book purge " + string(id) + "` before reusing the id")
		}
	}
	data, encodeErr := manifest.Encode()
	if encodeErr != nil {
		return NewBookResult{}, cli.AsError(encodeErr)
	}
	container := store.New()
	if writeErr := container.Write(lifecycle.ManifestMember, data); writeErr != nil {
		return NewBookResult{}, cli.AsError(writeErr)
	}
	if saveErr := container.Save(paths.Container); saveErr != nil {
		return NewBookResult{}, cli.AsError(saveErr)
	}
	registry.Upsert(store.RegistryEntry{ID: id, Path: outputPath, Kind: kind})
	if saveErr := registry.Save(service.Layout.Registry()); saveErr != nil {
		return NewBookResult{}, cli.AsError(saveErr)
	}
	result.Written = true

	renderService, prepareErr := service.preparedRender(registry, store.RegistryEntry{
		ID: id, Path: outputPath, Kind: kind,
	})
	if prepareErr != nil {
		return result, prepareErr
	}
	syncResult, syncErr := renderService.Sync(id, service.target(outputPath), render.SyncOptions{
		Write: true, Force: options.Force,
	})
	if syncErr != nil {
		return result, cli.AsError(syncErr)
	}
	result.Files = syncResult.Files
	if kind == store.KindBook {
		if refreshErr := service.refreshLibrary(registry, options.Force); refreshErr != nil {
			return result, refreshErr
		}
	}
	return result, nil
}

// MoveOptions is one `book move` request.
type MoveOptions struct {
	ID           model.BookID
	Path         string
	AllowOutside bool
}

// MoveResult is the receipt of `book move`.
type MoveResult struct {
	DryRun  bool   `json:"dryRun"`
	ID      string `json:"id"`
	From    string `json:"from"`
	To      string `json:"to"`
	Changed bool   `json:"changed"`
}

// DryRunResult marks the payload as a dry-run receipt.
func (result MoveResult) DryRunResult() bool { return result.DryRun }

func (result MoveResult) Text() string {
	if result.DryRun {
		return fmt.Sprintf("dry run: move %s from %s to %s", result.ID, result.From, result.To)
	}
	return fmt.Sprintf("%s: moved output path from %s to %s (run `pecia sync --write`)",
		result.ID, result.From, result.To)
}

// Move changes the registry output path. It never touches the container or the bodies and refuses an
// open book.
func (service *Service) Move(options MoveOptions, dryRun bool) (MoveResult, *cli.Error) {
	clean, pathErr := store.RegistryPath(options.Path)
	if pathErr != nil {
		return MoveResult{}, cli.Usage("usage.invalid-value", "%v", pathErr)
	}
	if !options.AllowOutside {
		if outsideErr := service.outsideDocs(clean); outsideErr != nil {
			return MoveResult{}, outsideErr
		}
	}
	storeRelease, lockErr := service.Life.LockStore()
	if lockErr != nil {
		return MoveResult{}, cli.AsError(lockErr)
	}
	defer func() { _ = storeRelease() }()
	bookRelease, bookLockErr := service.Life.LockBookExclusive(options.ID)
	if bookLockErr != nil {
		return MoveResult{}, cli.AsError(bookLockErr)
	}
	defer func() { _ = bookRelease() }()
	if openErr := service.requireClosed(options.ID); openErr != nil {
		return MoveResult{}, openErr
	}
	registry, registryErr := service.loadRegistry()
	if registryErr != nil {
		return MoveResult{}, registryErr
	}
	entry, ok := registry.Find(options.ID)
	if !ok {
		return MoveResult{}, unknownBook(options.ID)
	}
	if entry.Kind == store.KindLibrary {
		return MoveResult{}, immovable(entry.ID)
	}
	for _, other := range registry.Books {
		if other.ID == entry.ID {
			continue
		}
		if otherClean, _ := store.RegistryPath(other.Path); store.PathKey(otherClean) == store.PathKey(clean) {
			return MoveResult{}, cli.Reject("library.path-in-use",
				"output path %q is already used by book %q", clean, other.ID)
		}
	}
	result := MoveResult{
		DryRun: dryRun, ID: string(entry.ID), From: entry.Path, To: clean, Changed: entry.Path != clean,
	}
	if dryRun || !result.Changed {
		return result, nil
	}
	entry.Path = clean
	registry.Upsert(entry)
	if saveErr := registry.Save(service.Layout.Registry()); saveErr != nil {
		return MoveResult{}, cli.AsError(saveErr)
	}
	if renderErr := service.renderBookAt(registry, entry); renderErr != nil {
		return result, renderErr
	}
	if refreshErr := service.refreshLibrary(registry, false); refreshErr != nil {
		return result, refreshErr
	}
	return result, nil
}

// RenameOptions is one `book rename` request.
type RenameOptions struct {
	ID    model.BookID
	Title string
}

// RenameResult is the receipt of `book rename`.
type RenameResult struct {
	DryRun  bool   `json:"dryRun"`
	ID      string `json:"id"`
	From    string `json:"from"`
	To      string `json:"to"`
	Changed bool   `json:"changed"`
}

// DryRunResult marks the payload as a dry-run receipt.
func (result RenameResult) DryRunResult() bool { return result.DryRun }

func (result RenameResult) Text() string {
	if result.DryRun {
		return fmt.Sprintf("dry run: rename %s from %q to %q", result.ID, result.From, result.To)
	}
	return fmt.Sprintf("%s: renamed from %q to %q (run `pecia sync --write`)",
		result.ID, result.From, result.To)
}

// Rename changes the book title inside the closed container. The registry path is unchanged.
func (service *Service) Rename(options RenameOptions, dryRun bool) (RenameResult, *cli.Error) {
	title := model.NormalizeTitle(options.Title)
	if strings.TrimSpace(title) == "" {
		return RenameResult{}, cli.Usage("usage.missing-argument", "--title is required")
	}
	release, lockErr := service.Life.LockBookExclusive(options.ID)
	if lockErr != nil {
		return RenameResult{}, cli.AsError(lockErr)
	}
	defer func() { _ = release() }()
	if openErr := service.requireClosed(options.ID); openErr != nil {
		return RenameResult{}, openErr
	}
	registry, registryErr := service.loadRegistry()
	if registryErr != nil {
		return RenameResult{}, registryErr
	}
	entry, registered := registry.Find(options.ID)
	if !registered {
		return RenameResult{}, unknownBook(options.ID)
	}
	paths, pathsErr := service.Layout.Book(options.ID)
	if pathsErr != nil {
		return RenameResult{}, cli.AsError(pathsErr)
	}
	container, loadErr := store.Load(paths.Container)
	if loadErr != nil {
		return RenameResult{}, cli.AsError(loadErr)
	}
	data, ok := container.Read(lifecycle.ManifestMember)
	if !ok {
		return RenameResult{}, cli.Fail("library.manifest-missing",
			"book %q has no %s member", options.ID, lifecycle.ManifestMember)
	}
	manifest, decodeErr := model.Decode(data)
	if decodeErr != nil {
		return RenameResult{}, cli.AsError(decodeErr)
	}
	from := manifest.Root.Title
	result := RenameResult{
		DryRun: dryRun, ID: string(options.ID), From: from, To: title, Changed: from != title,
	}
	if dryRun || !result.Changed {
		return result, nil
	}
	manifest.Root.Title = title
	if validationErr := manifest.Validate(); validationErr != nil {
		return RenameResult{}, cli.AsError(validationErr)
	}
	encoded, encodeErr := manifest.Encode()
	if encodeErr != nil {
		return RenameResult{}, cli.AsError(encodeErr)
	}
	if writeErr := container.Write(lifecycle.ManifestMember, encoded); writeErr != nil {
		return RenameResult{}, cli.AsError(writeErr)
	}
	if saveErr := container.Save(paths.Container); saveErr != nil {
		return RenameResult{}, cli.AsError(saveErr)
	}
	if renderErr := service.renderBookAt(registry, entry); renderErr != nil {
		return result, renderErr
	}
	if entry.Kind != store.KindLibrary {
		if refreshErr := service.refreshLibrary(registry, false); refreshErr != nil {
			return result, refreshErr
		}
	}
	return result, nil
}

// UnregisterResult is the receipt of `book unregister`.
type UnregisterResult struct {
	DryRun  bool   `json:"dryRun"`
	ID      string `json:"id"`
	Path    string `json:"path"`
	Changed bool   `json:"changed"`
	Deleted bool   `json:"deleted"`
}

// DryRunResult marks the payload as a dry-run receipt.
func (result UnregisterResult) DryRunResult() bool { return result.DryRun }

func (result UnregisterResult) Text() string {
	if result.DryRun {
		return fmt.Sprintf("dry run: unregister %s (output stays at %s)", result.ID, result.Path)
	}
	return fmt.Sprintf("%s: unregistered; nothing deleted, output stays at %s", result.ID, result.Path)
}

// Unregister removes the registry entry and deletes nothing.
func (service *Service) Unregister(id model.BookID, dryRun bool) (UnregisterResult, *cli.Error) {
	storeRelease, lockErr := service.Life.LockStore()
	if lockErr != nil {
		return UnregisterResult{}, cli.AsError(lockErr)
	}
	defer func() { _ = storeRelease() }()
	bookRelease, bookLockErr := service.Life.LockBookExclusive(id)
	if bookLockErr != nil {
		return UnregisterResult{}, cli.AsError(bookLockErr)
	}
	defer func() { _ = bookRelease() }()
	if openErr := service.requireClosed(id); openErr != nil {
		return UnregisterResult{}, openErr
	}
	registry, registryErr := service.loadRegistry()
	if registryErr != nil {
		return UnregisterResult{}, registryErr
	}
	entry, ok := registry.Find(id)
	if !ok {
		return UnregisterResult{}, unknownBook(id)
	}
	if entry.Kind == store.KindLibrary {
		return UnregisterResult{}, cli.Reject("library.immovable",
			"the library book %q cannot be unregistered", id)
	}
	result := UnregisterResult{DryRun: dryRun, ID: string(id), Path: entry.Path, Changed: true, Deleted: false}
	if dryRun {
		return result, nil
	}
	registry.Remove(id)
	if saveErr := registry.Save(service.Layout.Registry()); saveErr != nil {
		return UnregisterResult{}, cli.AsError(saveErr)
	}
	if refreshErr := service.refreshLibrary(registry, false); refreshErr != nil {
		return result, refreshErr
	}
	return result, nil
}

// PurgeResult is the receipt of `book purge`.
type PurgeResult struct {
	DryRun   bool     `json:"dryRun"`
	ID       string   `json:"id"`
	Removed  bool     `json:"removed"`
	Store    bool     `json:"store"`
	Rollback bool     `json:"rollback"`
	Temp     bool     `json:"temp"`
	Locks    int      `json:"locks"`
	Paths    []string `json:"paths"`
}

// DryRunResult marks the payload as a dry-run receipt.
func (result PurgeResult) DryRunResult() bool { return result.DryRun }

func (result PurgeResult) Text() string {
	if !result.Removed {
		if result.DryRun {
			return fmt.Sprintf("dry run: nothing to purge for %s", result.ID)
		}
		return fmt.Sprintf("%s: nothing to purge", result.ID)
	}
	parts := make([]string, 0, 4)
	if result.Store {
		parts = append(parts, "store")
	}
	if result.Rollback {
		parts = append(parts, "rollback")
	}
	if result.Temp {
		parts = append(parts, "temp")
	}
	if result.Locks > 0 {
		parts = append(parts, fmt.Sprintf("%d lock file(s)", result.Locks))
	}
	if result.DryRun {
		return fmt.Sprintf("dry run: purge %s (%s); the rendered output is untouched",
			result.ID, strings.Join(parts, ", "))
	}
	return fmt.Sprintf("%s: purged %s; the rendered output is untouched",
		result.ID, strings.Join(parts, ", "))
}

// Purge deletes the working data of an unregistered book: its store directory, rollback pool,
// coordinator lock files and temp inputs. It never touches the rendered output and refuses a
// registered, open or store-reserved book. The command is idempotent.
func (service *Service) Purge(id model.BookID, dryRun bool) (PurgeResult, *cli.Error) {
	result := PurgeResult{DryRun: dryRun, ID: string(id)}
	if !id.Valid() {
		return result, cli.Usage("usage.invalid-value", "invalid book id %q", id)
	}
	if store.ReservedBookID(id) {
		return result, cli.Reject("library.purge-reserved",
			"%q is part of the store layout and cannot be purged", id)
	}
	if store.AmbiguousBookID(id) {
		return result, cli.Reject("library.purge-ambiguous",
			"%q is not a plain store name and may alias another path", id).
			WithHint("use the exact book id without trailing dots or reserved characters")
	}
	storeRelease, lockErr := service.Life.LockStore()
	if lockErr != nil {
		return result, cli.AsError(lockErr)
	}
	defer func() { _ = storeRelease() }()

	registry, registryErr := service.loadRegistry()
	if registryErr != nil {
		return result, registryErr
	}
	if _, ok := registry.Library(); !ok {
		return result, cli.Reject("library.incomplete-registry",
			"the registry has no library entry; refusing to purge store data")
	}
	if closedErr := service.requireClosed(id); closedErr != nil {
		return result, closedErr
	}
	// Compare case-insensitively: on a case-insensitive file system an id like "LIBRARY" names
	// the same store directory as the registered "library" entry. An exact lookup would miss it
	// and purge a live book.
	for _, entry := range registry.Books {
		if !strings.EqualFold(string(entry.ID), string(id)) {
			continue
		}
		if entry.Kind == store.KindLibrary {
			return result, cli.Reject("library.immovable", "the library book %q cannot be purged", entry.ID)
		}
		return result, cli.Reject("library.book-registered", "book %q is still registered", entry.ID).
			WithHint("run `pecia book unregister " + string(entry.ID) + "` first")
	}
	paths, pathsErr := service.Layout.Book(id)
	if pathsErr != nil {
		return result, cli.AsError(pathsErr)
	}
	storeExists, storeErr := dirExists(paths.Dir)
	if storeErr != nil {
		return result, cli.Fail("library.purge-read-failed", "%v", storeErr)
	}
	rollbackExists, rollbackErr := dirExists(paths.RollbackDir)
	if rollbackErr != nil {
		return result, cli.Fail("library.purge-read-failed", "%v", rollbackErr)
	}
	tempDir := ""
	tempExists := false
	if service.Values != nil && service.Values.TempRoot != "" {
		tempDir = filepath.Join(service.Values.TempRoot, string(id))
		tempExists, storeErr = dirExists(tempDir)
		if storeErr != nil {
			return result, cli.Fail("library.purge-read-failed", "%v", storeErr)
		}
	}
	lockFiles, lockFilesErr := service.bookLockFiles(id)
	if lockFilesErr != nil {
		return result, lockFilesErr
	}
	result.Store = storeExists
	result.Rollback = rollbackExists
	result.Temp = tempExists
	result.Locks = len(lockFiles)
	result.Paths = make([]string, 0, 3)
	if storeExists {
		result.Paths = append(result.Paths, paths.Dir)
	}
	if rollbackExists {
		result.Paths = append(result.Paths, paths.RollbackDir)
	}
	if tempExists {
		result.Paths = append(result.Paths, tempDir)
	}
	result.Removed = len(result.Paths) > 0 || result.Locks > 0
	if dryRun || !result.Removed {
		return result, nil
	}
	// The store lock serializes every lifecycle and registry transition, and an unregistered book
	// cannot be opened or structurally mutated. Holding the book lock as well closes the remaining
	// window against a stray lock holder before its lock file is removed.
	bookRelease, bookLockErr := service.Life.LockBookExclusive(id)
	if bookLockErr != nil {
		return result, cli.AsError(bookLockErr)
	}
	bookLockPath := filepath.Join(service.Layout.Coordinator(), "book-"+string(id)+".lock")
	defer func() { _ = os.Remove(bookLockPath) }()
	defer func() { _ = bookRelease() }()
	if storeExists {
		if err := os.RemoveAll(paths.Dir); err != nil {
			return result, cli.Fail("library.purge-failed", "remove %s: %v", paths.Dir, err)
		}
	}
	if rollbackExists {
		if err := os.RemoveAll(paths.RollbackDir); err != nil {
			return result, cli.Fail("library.purge-failed", "remove %s: %v", paths.RollbackDir, err)
		}
	}
	if tempExists {
		if err := os.RemoveAll(tempDir); err != nil {
			return result, cli.Fail("library.purge-failed", "remove %s: %v", tempDir, err)
		}
	}
	_ = bookRelease()
	for _, lockFile := range lockFiles {
		if samePath(lockFile, bookLockPath) {
			continue
		}
		if err := os.Remove(lockFile); err != nil && !errors.Is(err, os.ErrNotExist) {
			return result, cli.Fail("library.purge-failed", "remove %s: %v", lockFile, err)
		}
	}
	return result, nil
}

// dirExists reports whether path is an existing directory. A non-directory at a store path is an
// error rather than a removal target, so a book id that collides with a registry file cannot
// delete that file.
func dirExists(path string) (bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.IsDir() {
		return false, fmt.Errorf("%s is not a directory", path)
	}
	return true, nil
}

// bookLockFiles returns the coordinator lock files of one book. The block and node suffix must be
// numeric so a book id that is a prefix of another id cannot match the other book's locks.
func (service *Service) bookLockFiles(id model.BookID) ([]string, *cli.Error) {
	directory := service.Layout.Coordinator()
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, cli.Fail("library.purge-read-failed", "%v", err)
	}
	fold := runtime.GOOS == "windows"
	bookName := "book-" + string(id) + ".lock"
	blockPrefix := "block-" + string(id) + "-"
	nodePrefix := "node-" + string(id) + "-"
	if fold {
		bookName = strings.ToLower(bookName)
		blockPrefix = strings.ToLower(blockPrefix)
		nodePrefix = strings.ToLower(nodePrefix)
	}
	files := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		compare := name
		if fold {
			compare = strings.ToLower(name)
		}
		switch {
		case compare == bookName:
			files = append(files, filepath.Join(directory, name))
		case strings.HasPrefix(compare, blockPrefix) && numericLock(compare[len(blockPrefix):]):
			files = append(files, filepath.Join(directory, name))
		case strings.HasPrefix(compare, nodePrefix) && numericLock(compare[len(nodePrefix):]):
			files = append(files, filepath.Join(directory, name))
		}
	}
	return files, nil
}

func samePath(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func numericLock(suffix string) bool {
	if !strings.HasSuffix(suffix, ".lock") {
		return false
	}
	_, err := strconv.ParseUint(strings.TrimSuffix(suffix, ".lock"), 10, 64)
	return err == nil
}

// refreshLibrary re-renders the library book so its Document Index follows the registry.
func (service *Service) refreshLibrary(registry *store.Registry, force bool) *cli.Error {
	entry, ok := registry.Library()
	if !ok {
		return nil
	}
	renderService, prepareErr := service.preparedRender(registry, entry)
	if prepareErr != nil {
		return prepareErr
	}
	_, syncErr := renderService.Sync(entry.ID, service.target(entry.Path), render.SyncOptions{
		Write: true, Force: force,
	})
	if syncErr != nil {
		return cli.AsError(syncErr)
	}
	return nil
}

// preparedRender wires the render service for one book: navigation, cross-book resolvers and, for
// the library book, the Document Index.
func (service *Service) preparedRender(registry *store.Registry, entry store.RegistryEntry) (*render.Service, *cli.Error) {
	renderService := service.renderService()
	index, indexErr := service.Index()
	if indexErr != nil {
		return nil, indexErr
	}
	if entry.Kind == store.KindLibrary {
		renderService.LibraryIndex = index
	}
	link, book, crossErr := renderService.CrossResolvers(registry, entry.ID, entry.Path)
	if crossErr != nil {
		return nil, crossErr
	}
	renderService.CrossLink, renderService.CrossBook = link, book
	renderService.Navigation = render.NavigationFor(registry, index, entry.ID, entry.Path)
	return renderService, nil
}

func (service *Service) renderService() *render.Service {
	renderService := render.New(service.Life)
	if service.Values != nil {
		renderService.Overrides = service.Values.AsciiUITypes
	}
	return renderService
}

// renderBookAt renders one registered book at its registered path.
func (service *Service) renderBookAt(registry *store.Registry, entry store.RegistryEntry) *cli.Error {
	renderService, prepareErr := service.preparedRender(registry, entry)
	if prepareErr != nil {
		return prepareErr
	}
	_, syncErr := renderService.Sync(entry.ID, service.target(entry.Path), render.SyncOptions{Write: true})
	if syncErr != nil {
		return cli.AsError(syncErr)
	}
	return nil
}

// target resolves a registry output path against the project directory.
func (service *Service) target(outputPath string) string {
	return filepath.Join(service.Values.ProjectDir, filepath.FromSlash(outputPath))
}

func dirIsEmpty(directory string) (bool, error) {
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return len(entries) == 0, nil
}

// pathSlug resolves the docs-root-relative default path of a book.
func defaultBookPath(service *Service, id model.BookID) (string, *cli.Error) {
	relative, relErr := filepath.Rel(service.Values.ProjectDir, service.Values.DocsRoot)
	if relErr != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", cli.Reject("library.docs-root-outside",
			"the docs root %s is outside the project", service.Values.DocsRoot).
			WithHint("set docs.root inside the project or pass --path")
	}
	return path.Clean(path.Join(filepath.ToSlash(relative), string(id))), nil
}
