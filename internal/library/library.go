package library

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/config"
	"github.com/Blue-Alien-Tec/clabok/internal/idspace"
	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

// Service is the registry-backed library.
type Service struct {
	Life   *lifecycle.Lifecycle
	Layout store.Layout
	Values *config.Values
	IDs    *idspace.Allocator
}

// FromEnv builds the library service from the command environment.
func FromEnv(env *cli.Env) (*Service, *cli.Error) {
	life, layout, err := lifecycle.FromEnv(env)
	if err != nil {
		return nil, err
	}
	values, configErr := config.Load(env)
	if configErr != nil {
		return nil, configErr
	}
	return &Service{Life: life, Layout: layout, Values: values, IDs: idspace.New(life, layout)}, nil
}

func (service *Service) loadRegistry() (*store.Registry, *cli.Error) {
	registry, err := store.LoadRegistry(service.Layout.Registry())
	if err != nil {
		return nil, cli.AsError(err)
	}
	return registry, nil
}

func (service *Service) requireClosed(id model.BookID) *cli.Error {
	if err := service.Life.RequireClosed(id); err != nil {
		return cli.Reject("library.book-open", "book %q is open; close it first", id).
			WithHint("run `pecia book close " + string(id) + "`")
	}
	return nil
}

func (service *Service) newBookID(registry *store.Registry, requested, title string) (model.BookID, *cli.Error) {
	if requested != "" {
		id := model.BookID(requested)
		if idErr := store.ValidateBookID(id); idErr != nil {
			return "", cli.Reject("library.id-invalid", "%v", idErr)
		}
		if entry, taken := registry.FindFold(id); taken {
			return "", cli.Reject("library.id-in-use",
				"book id %q is already registered as %q", requested, entry.ID)
		}
		return id, nil
	}
	base := Slug(title)
	for suffix := 1; ; suffix++ {
		candidate := base
		if suffix > 1 {
			candidate = base + "-" + strconv.Itoa(suffix)
		}
		id := model.BookID(candidate)
		if idErr := store.ValidateBookID(id); idErr != nil {
			return "", cli.Reject("library.id-unavailable",
				"cannot derive a usable book id from %q", title).
				WithHint("pass --id with a plain id")
		}
		if _, taken := registry.FindFold(id); !taken {
			return id, nil
		}
	}
}

func (service *Service) newBookPath(registry *store.Registry, options NewBookOptions, id model.BookID) (string, *cli.Error) {
	if options.Library {
		return ".", nil
	}
	var outputPath string
	if options.Path != "" {
		clean, pathErr := store.RegistryPath(options.Path)
		if pathErr != nil {
			return "", cli.Usage("usage.invalid-value", "%v", pathErr)
		}
		outputPath = clean
	} else {
		defaultPath, defaultErr := defaultBookPath(service, id)
		if defaultErr != nil {
			return "", defaultErr
		}
		outputPath = defaultPath
	}
	if outputPath == "." {
		return "", cli.Reject("library.path-conflict",
			"output path \".\" is reserved for the library book").
			WithHint("pass a subdirectory such as --path Docs/" + string(id))
	}
	if !options.AllowOutside {
		if outsideErr := service.outsideDocs(outputPath); outsideErr != nil {
			return "", outsideErr
		}
	}
	for _, entry := range registry.Books {
		if store.PathKey(entry.Path) == store.PathKey(outputPath) {
			return "", cli.Reject("library.path-in-use",
				"output path %q is already used by book %q", outputPath, entry.ID)
		}
	}
	return outputPath, nil
}

// outsideDocs rejects a path outside the docs root unless the caller allowed it.
func (service *Service) outsideDocs(outputPath string) *cli.Error {
	target := filepath.Clean(filepath.Join(service.Values.ProjectDir, filepath.FromSlash(outputPath)))
	docsRoot := filepath.Clean(service.Values.DocsRoot)
	relative, err := filepath.Rel(docsRoot, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return cli.Reject("library.outside-docs-root",
			"output path %q is outside the docs root %s", outputPath, docsRoot).
			WithHint("pass --allow-outside to accept a different location")
	}
	return nil
}

func effectiveSettings(registry *store.Registry, options NewBookOptions) (model.Settings, *cli.Error) {
	settings := registry.EffectiveDefaults()
	if options.Numbering != nil {
		settings.Numbering.Enabled = *options.Numbering
	}
	if options.TOC != nil {
		settings.TOC.Enabled = *options.TOC
	}
	if options.IndexDepth != nil {
		settings.IndexDepth = *options.IndexDepth
	}
	if options.Output != "" {
		mode := model.OutputMode(strings.TrimSpace(options.Output))
		if !mode.Valid() {
			return model.Settings{}, cli.Usage("usage.invalid-value",
				"output expects monolith, chapter or part, got %q", options.Output)
		}
		settings.Output = mode
	}
	settings = settings.Normalized()
	if options.IndexDepth != nil && *options.IndexDepth < 1 {
		return model.Settings{}, cli.Usage("usage.invalid-value",
			"index-depth expects an integer of at least 1")
	}
	return settings, nil
}

func unknownBook(id model.BookID) *cli.Error {
	return cli.Reject("library.unknown-book", "book %q is not registered", id).
		WithHint("run `pecia sync` to list the registered books")
}

func immovable(id model.BookID) *cli.Error {
	return cli.Reject("library.immovable", "the library book %q has the fixed output path \".\"", id)
}

// Slug derives a book ID from a title: lowercase ASCII words joined by single dashes.
func Slug(title string) string {
	var builder strings.Builder
	lastDash := false
	for _, character := range strings.ToLower(title) {
		switch {
		case character >= 'a' && character <= 'z', character >= '0' && character <= '9':
			builder.WriteRune(character)
			lastDash = false
		default:
			if !lastDash && builder.Len() > 0 {
				builder.WriteByte('-')
				lastDash = true
			}
		}
	}
	value := strings.Trim(builder.String(), "-")
	if value == "" {
		return "book"
	}
	return value
}
