package render

import (
	"errors"
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/config"
	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

// CatalogFunc supplies the library Document Index for the library book. It is the consumer-side
// interface of the library package; render never imports library.
type CatalogFunc func(*cli.Env) ([]IndexEntry, *cli.Error)

// catalog is the wired library index provider. It is nil in tests that do not need a library.
var catalog CatalogFunc

type syncPayload struct {
	DryRun  bool     `json:"dryRun"`
	Book    string   `json:"book"`
	Out     string   `json:"out"`
	Mode    string   `json:"mode"`
	Status  string   `json:"status"`
	Files   []string `json:"files"`
	Changed []string `json:"changed,omitempty"`
	Foreign []string `json:"foreign,omitempty"`
	Pruned  []string `json:"pruned,omitempty"`
	Written bool     `json:"written"`
}

// DryRunResult marks the payload as a dry-run receipt.
func (payload syncPayload) DryRunResult() bool { return payload.DryRun }

func (payload syncPayload) Text() string {
	state := "checked"
	if payload.Written {
		state = "written"
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "%s: %s (%d file(s), %d changed, %d pruned) %s\n",
		payload.Book, payload.Status, len(payload.Files), len(payload.Changed), len(payload.Pruned), state)
	for _, name := range payload.Changed {
		fmt.Fprintf(&builder, "  changed %s\n", name)
	}
	for _, name := range payload.Foreign {
		fmt.Fprintf(&builder, "  foreign %s\n", name)
	}
	return builder.String()
}

type libraryBookError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type libraryBookPayload struct {
	ID      string            `json:"id"`
	Path    string            `json:"path"`
	Kind    string            `json:"kind"`
	Mode    string            `json:"mode,omitempty"`
	Status  string            `json:"status,omitempty"`
	Files   []string          `json:"files,omitempty"`
	Changed []string          `json:"changed,omitempty"`
	Foreign []string          `json:"foreign,omitempty"`
	Pruned  []string          `json:"pruned,omitempty"`
	Written bool              `json:"written"`
	Error   *libraryBookError `json:"error,omitempty"`
}

type librarySyncPayload struct {
	DryRun bool                 `json:"dryRun"`
	Check  bool                 `json:"check"`
	Clean  bool                 `json:"clean"`
	Books  []libraryBookPayload `json:"books"`
}

// DryRunResult marks the payload as a dry-run receipt.
func (payload librarySyncPayload) DryRunResult() bool { return payload.DryRun }

func (payload librarySyncPayload) Text() string {
	var builder strings.Builder
	changed := 0
	for _, book := range payload.Books {
		if book.Error != nil {
			changed++
			fmt.Fprintf(&builder, "%s: error %s (%s)\n", book.ID, book.Error.Code, book.Error.Message)
			continue
		}
		if len(book.Changed) > 0 || len(book.Foreign) > 0 || len(book.Pruned) > 0 {
			changed++
		}
		state := "clean"
		if book.Written {
			state = "written"
		} else if len(book.Changed) > 0 {
			state = "drift"
		}
		fmt.Fprintf(&builder, "%s: %s (%d file(s), %d changed)\n",
			book.ID, state, len(book.Files), len(book.Changed))
	}
	fmt.Fprintf(&builder, "%d book(s), %d with changes, clean=%t\n",
		len(payload.Books), changed, payload.Clean)
	return builder.String()
}

type checkPayload struct {
	Book     string    `json:"book"`
	Findings []Finding `json:"findings"`
	Blocking int       `json:"blocking"`
	Clean    bool      `json:"clean"`
}

func (payload checkPayload) Text() string {
	var builder strings.Builder
	for _, finding := range payload.Findings {
		level := "info"
		if finding.Blocking {
			level = "error"
		}
		fmt.Fprintf(&builder, "%s %s: %s\n", level, finding.Code, finding.Message)
	}
	fmt.Fprintf(&builder, "%d finding(s), %d blocking\n", len(payload.Findings), payload.Blocking)
	return builder.String()
}

// Register adds the render, sync and check commands. catalog may be nil; it is required only for a
// library book to carry its Document Index.
func Register(registry *cli.Registry, index CatalogFunc) {
	catalog = index
	registry.Add(syncCommand())
	registry.Add(checkCommand("check"))
	registry.Add(checkCommand("verify"))
}

func syncCommand() *cli.Command {
	var book, out string
	var check, write, prune, force bool
	return &cli.Command{
		Name:     "sync",
		Summary:  "render the book and compare or write the output",
		Usage:    "pecia sync [--book <id>] [--out <dir>] [--check | --write] [--prune] [--force]",
		Mutating: true,
		Flags: func(env *cli.Env) *flag.FlagSet {
			set := env.NewFlagSet("sync")
			set.StringVar(&book, "book", "", "book id; omit to sync every registered book")
			set.StringVar(&out, "out", "", "output directory (defaults to the registry path)")
			set.BoolVar(&check, "check", false, "report drift without writing and fail on drift")
			set.BoolVar(&write, "write", false, "write changed files")
			set.BoolVar(&prune, "prune", false, "remove generated files the mode no longer produces")
			set.BoolVar(&force, "force", false, "overwrite files without the generated marker")
			return set
		},
		Run: func(env *cli.Env, args []string) (*cli.Result, error) {
			if len(args) != 0 {
				return nil, cli.Usage("usage.unexpected-argument", "sync takes flags only")
			}
			if check && write {
				return nil, cli.Usage("usage.invalid-arguments", "use --check or --write, not both")
			}
			if book == "" {
				if out != "" {
					return nil, cli.Usage("usage.invalid-arguments", "--out needs --book")
				}
				return unwrap(RunLibrarySync(env, check, write, prune, force))
			}
			bookID, bookErr := requireBook(book)
			if bookErr != nil {
				return nil, bookErr
			}
			return unwrap(runSyncBook(env, bookID, out, check, write, prune, force))
		},
	}
}

// unwrap keeps a nil *cli.Error from becoming a non-nil error interface.
func unwrap(res *cli.Result, err *cli.Error) (*cli.Result, error) {
	if err != nil {
		return nil, err
	}
	return res, nil
}

func runSyncBook(env *cli.Env, book model.BookID, out string, check, write, prune, force bool) (*cli.Result, *cli.Error) {
	service, serviceErr := newService(env)
	if serviceErr != nil {
		return nil, serviceErr
	}
	target, entry, registered, targetErr := outputDir(env, book, out)
	if targetErr != nil {
		return nil, targetErr
	}
	values, configErr := config.Load(env)
	if configErr != nil {
		return nil, configErr
	}
	layout, layoutErr := store.NewLayout(values.StoreRoot)
	if layoutErr != nil {
		return nil, cli.AsError(layoutErr)
	}
	registry, registryErr := store.LoadRegistry(layout.Registry())
	if registryErr != nil {
		return nil, cli.AsError(registryErr)
	}
	cross, crossErr := buildCrossIndex(service, registry)
	if crossErr != nil {
		return nil, crossErr
	}
	service.CrossLink, service.CrossBook = cross.forBook(book, relativeDir(values.ProjectDir, target))
	service.Navigation = NavigationFor(registry, book, relativeDir(values.ProjectDir, target))
	if registered && entry.Kind == store.KindLibrary && catalog != nil {
		index, indexErr := catalog(env)
		if indexErr != nil {
			return nil, indexErr
		}
		service.LibraryIndex = index
	}
	dryRun := !write || env.Globals.DryRun
	syncResult, syncErr := service.Sync(book, target, SyncOptions{
		Check: check || dryRun, Write: !dryRun, Prune: prune, Force: force,
	})
	if syncErr != nil {
		return nil, cli.AsError(syncErr)
	}
	payload := syncPayload{
		DryRun: dryRun, Book: string(book), Out: target, Mode: string(syncResult.Mode),
		Status: string(syncResult.Status), Files: syncResult.Files, Changed: syncResult.Changed,
		Foreign: syncResult.Foreign, Pruned: syncResult.Pruned, Written: syncResult.Written,
	}
	if check && !env.Globals.DryRun && !cleanBook(syncResult.Changed, syncResult.Foreign, syncResult.Pruned) {
		return nil, cli.Fail("render.drift", "book %q has drift", book).
			WithHint("run `pecia sync --book " + string(book) + " --write`").
			WithDetails(payload)
	}
	return &cli.Result{Data: payload}, nil
}

// RunLibrarySync renders every registered book. It is shared by `sync` without --book and by
// `library sync`. A real per-book failure is reported in the payload and fails the command after
// every book has been assessed; drift fails only an explicit --check.
func RunLibrarySync(env *cli.Env, check, write, prune, force bool) (*cli.Result, *cli.Error) {
	values, configErr := config.Load(env)
	if configErr != nil {
		return nil, configErr
	}
	layout, layoutErr := store.NewLayout(values.StoreRoot)
	if layoutErr != nil {
		return nil, cli.AsError(layoutErr)
	}
	registry, registryErr := store.LoadRegistry(layout.Registry())
	if registryErr != nil {
		return nil, cli.AsError(registryErr)
	}
	if len(registry.Books) == 0 {
		return nil, cli.Fail("library.empty", "no books are registered").
			WithHint("run `pecia book new --title <title>`")
	}
	service, serviceErr := newService(env)
	if serviceErr != nil {
		return nil, serviceErr
	}
	if idErr := checkLibraryIDs(service, registry); idErr != nil {
		return nil, idErr
	}
	cross, crossErr := buildCrossIndex(service, registry)
	if crossErr != nil {
		return nil, crossErr
	}
	var index []IndexEntry
	if _, hasLibrary := registry.Library(); hasLibrary && catalog != nil {
		built, indexErr := catalog(env)
		if indexErr != nil {
			return nil, indexErr
		}
		index = built
	}
	dryRun := !write || env.Globals.DryRun
	options := SyncOptions{Check: check || dryRun, Write: !dryRun, Prune: prune, Force: force}
	payload := librarySyncPayload{DryRun: dryRun, Check: check}
	clean := true
	for _, entry := range renderOrder(registry.Books) {
		target := filepath.Join(values.ProjectDir, filepath.FromSlash(entry.Path))
		service.LibraryIndex = nil
		if entry.Kind == store.KindLibrary {
			service.LibraryIndex = index
		}
		service.CrossLink, service.CrossBook = cross.forBook(entry.ID, entry.Path)
		service.Navigation = NavigationFor(registry, entry.ID, entry.Path)
		bookPayload := libraryBookPayload{ID: string(entry.ID), Path: entry.Path, Kind: entry.Kind}
		syncResult, syncErr := service.Sync(entry.ID, target, options)
		if syncErr != nil {
			bookPayload.Error = &libraryBookError{Code: errorCode(syncErr), Message: syncErr.Error()}
			clean = false
			payload.Books = append(payload.Books, bookPayload)
			continue
		}
		bookPayload.Mode = string(syncResult.Mode)
		bookPayload.Status = string(syncResult.Status)
		bookPayload.Files = syncResult.Files
		bookPayload.Changed = syncResult.Changed
		bookPayload.Foreign = syncResult.Foreign
		bookPayload.Pruned = syncResult.Pruned
		bookPayload.Written = syncResult.Written
		if !syncClean(syncResult, prune) {
			clean = false
		}
		payload.Books = append(payload.Books, bookPayload)
	}
	payload.Clean = clean
	for _, bookPayload := range payload.Books {
		if bookPayload.Error != nil {
			return nil, cli.Fail("library.sync-failed", "at least one book could not be synced").
				WithDetails(payload)
		}
	}
	if check && !env.Globals.DryRun && !clean {
		return nil, cli.Fail("render.drift", "the library has drift").
			WithHint("run `pecia sync --write`").
			WithDetails(payload)
	}
	return &cli.Result{Data: payload}, nil
}

// renderOrder renders the non-library books first and the library book last, so the generated index
// reflects the final state. Registry order still determines the index rows.
func renderOrder(entries []store.RegistryEntry) []store.RegistryEntry {
	ordered := make([]store.RegistryEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Kind != store.KindLibrary {
			ordered = append(ordered, entry)
		}
	}
	for _, entry := range entries {
		if entry.Kind == store.KindLibrary {
			ordered = append(ordered, entry)
		}
	}
	return ordered
}

func cleanBook(changed, foreign, pruned []string) bool {
	return len(changed) == 0 && len(foreign) == 0 && len(pruned) == 0
}

func syncClean(result SyncResult, prune bool) bool {
	if len(result.Foreign) > 0 {
		return false
	}
	if !result.Written && len(result.Changed) > 0 {
		return false
	}
	if len(result.Pruned) > 0 && !(result.Written && prune) {
		return false
	}
	return true
}

func errorCode(err error) string {
	var coded interface{ ErrorCode() string }
	if errors.As(err, &coded) && coded.ErrorCode() != "" {
		return coded.ErrorCode()
	}
	return "render.sync-failed"
}

func checkCommand(name string) *cli.Command {
	var book string
	return &cli.Command{
		Name:    name,
		Summary: "run the read-only acceptance checks",
		Usage:   "pecia " + name + " --book <id>",
		Flags: func(env *cli.Env) *flag.FlagSet {
			set := env.NewFlagSet(name)
			set.StringVar(&book, "book", "", "book id")
			return set
		},
		Run: func(env *cli.Env, args []string) (*cli.Result, error) {
			if len(args) != 0 {
				return nil, cli.Usage("usage.unexpected-argument", "%s takes flags only", name)
			}
			bookID, bookErr := requireBook(book)
			if bookErr != nil {
				return nil, bookErr
			}
			service, serviceErr := newService(env)
			if serviceErr != nil {
				return nil, serviceErr
			}
			values, configErr := config.Load(env)
			if configErr != nil {
				return nil, configErr
			}
			layout, layoutErr := store.NewLayout(values.StoreRoot)
			if layoutErr != nil {
				return nil, cli.AsError(layoutErr)
			}
			registry, registryErr := store.LoadRegistry(layout.Registry())
			if registryErr != nil {
				return nil, cli.AsError(registryErr)
			}
			cross, crossErr := buildCrossIndex(service, registry)
			if crossErr != nil {
				return nil, crossErr
			}
			fromDir := ""
			if entry, registered := registry.Find(bookID); registered {
				fromDir = entry.Path
			}
			service.CrossLink, service.CrossBook = cross.forBook(bookID, fromDir)
			findings, checkErr := service.Check(bookID)
			if checkErr != nil {
				return nil, cli.AsError(checkErr)
			}
			blocking := 0
			for _, finding := range findings {
				if finding.Blocking {
					blocking++
				}
			}
			payload := checkPayload{
				Book: string(bookID), Findings: findings, Blocking: blocking, Clean: blocking == 0,
			}
			if blocking > 0 {
				return nil, cli.Fail("render.check-failed", "%d blocking finding(s)", blocking).
					WithHint("fix the reported findings and run the check again").
					WithDetails(payload)
			}
			return &cli.Result{Data: payload}, nil
		},
	}
}

func newService(env *cli.Env) (*Service, *cli.Error) {
	life, _, err := lifecycle.FromEnv(env)
	if err != nil {
		return nil, err
	}
	values, configErr := config.Load(env)
	if configErr != nil {
		return nil, configErr
	}
	service := New(life)
	service.Overrides = values.AsciiUITypes
	return service, nil
}

func relativeDir(projectDir, target string) string {
	rel, err := filepath.Rel(projectDir, target)
	if err != nil {
		return ""
	}
	return filepath.ToSlash(rel)
}

func requireBook(raw string) (model.BookID, *cli.Error) {
	if raw == "" {
		return "", cli.Usage("usage.missing-argument", "--book is required")
	}
	id := model.BookID(raw)
	if !id.Valid() {
		return "", cli.Usage("usage.invalid-value", "invalid book id %q", raw)
	}
	return id, nil
}

// outputDir resolves the target directory: --out, then the registry path. It also returns the
// registry entry so the caller can detect the library book.
func outputDir(env *cli.Env, book model.BookID, explicit string) (string, store.RegistryEntry, bool, *cli.Error) {
	values, err := config.Load(env)
	if err != nil {
		return "", store.RegistryEntry{}, false, err
	}
	layout, layoutErr := store.NewLayout(values.StoreRoot)
	if layoutErr != nil {
		return "", store.RegistryEntry{}, false, cli.AsError(layoutErr)
	}
	registry, registryErr := store.LoadRegistry(layout.Registry())
	if registryErr != nil {
		return "", store.RegistryEntry{}, false, cli.AsError(registryErr)
	}
	entry, registered := registry.Find(book)
	if explicit != "" {
		if filepath.IsAbs(explicit) {
			return filepath.Clean(explicit), entry, registered, nil
		}
		return filepath.Join(values.ProjectDir, explicit), entry, registered, nil
	}
	if !registered {
		return "", store.RegistryEntry{}, false, cli.Fail("render.not-registered",
			"book %q is not registered; pass --out <dir>", book).
			WithHint("pecia sync --book <id> --out <dir>")
	}
	return filepath.Join(values.ProjectDir, filepath.FromSlash(entry.Path)), entry, true, nil
}
