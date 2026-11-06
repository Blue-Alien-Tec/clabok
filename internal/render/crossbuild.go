package render

import (
	"errors"
	"os"
	"path"
	"path/filepath"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

// crossIndex is the library-wide reference map. It is built from the manifests only, so resolving a
// cross-book reference never renders or extracts another book.
type crossIndex struct {
	order   []model.BookID
	paths   map[model.BookID]string
	layouts map[model.BookID]map[model.ID]LinkTarget
}

// readManifest reads a book manifest without extracting or opening the book.
func (service *Service) readManifest(book model.BookID) (*model.Manifest, error) {
	paths, err := service.Lifecycle.Paths(book)
	if err != nil {
		return nil, err
	}
	state, stateErr := service.Lifecycle.State(book)
	if stateErr != nil {
		return nil, stateErr
	}
	if state.Extracted {
		data, readErr := os.ReadFile(filepath.Join(paths.Dir, lifecycle.ManifestMember))
		if readErr == nil {
			return model.Decode(data)
		}
		if !errors.Is(readErr, os.ErrNotExist) {
			return nil, readErr
		}
	}
	container, loadErr := store.Load(paths.Container)
	if loadErr != nil {
		return nil, loadErr
	}
	data, ok := container.Read(lifecycle.ManifestMember)
	if !ok {
		return nil, fail("render.manifest-missing",
			"book %q has no %s member", book, lifecycle.ManifestMember)
	}
	return model.Decode(data)
}

// buildCrossIndex reads every registered manifest and computes its rendered layout.
func buildCrossIndex(service *Service, registry *store.Registry) (*crossIndex, *cli.Error) {
	index := &crossIndex{
		order:   make([]model.BookID, 0, len(registry.Books)),
		paths:   make(map[model.BookID]string, len(registry.Books)),
		layouts: make(map[model.BookID]map[model.ID]LinkTarget, len(registry.Books)),
	}
	for _, entry := range registry.Books {
		manifest, err := service.readManifest(entry.ID)
		if err != nil {
			return nil, cli.AsError(err)
		}
		index.order = append(index.order, entry.ID)
		index.paths[entry.ID] = entry.Path
		index.layouts[entry.ID] = LayoutFor(manifest)
	}
	return index, nil
}

// CrossResolvers builds the library-wide reference resolvers for one book. It reads every registered
// manifest once and never opens or extracts a book.
func (service *Service) CrossResolvers(registry *store.Registry, current model.BookID, fromDir string) (CrossLinkFunc, CrossBookFunc, *cli.Error) {
	index, err := buildCrossIndex(service, registry)
	if err != nil {
		return nil, nil, err
	}
	link, book := index.forBook(current, fromDir)
	return link, book, nil
}

// NavigationFor returns the navigation context of one book. fromDir is its output directory relative
// to the project root; index is the library Document Index in registry order.
func NavigationFor(registry *store.Registry, index []IndexEntry, current model.BookID, fromDir string) Navigation {
	if registry == nil {
		return Navigation{}
	}
	library, ok := registry.Library()
	if !ok {
		return Navigation{}
	}
	currentEntry, _ := registry.Find(current)
	currentTarget := path.Clean(path.Join(currentEntry.Path, "README.md"))
	books := make([]BookLink, 0, len(index))
	for _, entry := range index {
		target := path.Clean(path.Join(library.Path, entry.Href))
		if current != library.ID && target == currentTarget {
			continue
		}
		books = append(books, BookLink{Title: entry.Title, Href: relHref(fromDir, target)})
	}
	if current == library.ID {
		return Navigation{IsLibrary: true, Books: books}
	}
	return Navigation{
		LibraryHref: relHref(fromDir, path.Join(library.Path, "README.md")),
		Books:       books,
	}
}

// forBook returns the resolvers for one book. fromDir is the book's output directory, relative to
// the project root.
func (index *crossIndex) forBook(book model.BookID, fromDir string) (CrossLinkFunc, CrossBookFunc) {
	link := func(id model.ID) (CrossLink, bool) {
		for _, candidate := range index.order {
			if candidate == book {
				continue
			}
			target, ok := index.layouts[candidate][id]
			if !ok || target.File == "" {
				continue
			}
			href := relHref(fromDir, path.Join(index.paths[candidate], target.Href()))
			return CrossLink{Title: target.Title, Href: href}, true
		}
		return CrossLink{}, false
	}
	bookLink := func(candidate model.BookID) (string, bool) {
		targetPath, ok := index.paths[candidate]
		if !ok {
			return "", false
		}
		if candidate == book {
			return "README.md", true
		}
		return relHref(fromDir, path.Join(targetPath, "README.md")), true
	}
	return link, bookLink
}
