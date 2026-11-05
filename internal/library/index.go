package library

import (
	"errors"
	"os"
	"path"
	"path/filepath"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/render"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

// Index returns the library Document Index in registry order, excluding the library itself.
func (service *Service) Index() ([]render.IndexEntry, *cli.Error) {
	registry, registryErr := service.loadRegistry()
	if registryErr != nil {
		return nil, registryErr
	}
	entries := make([]render.IndexEntry, 0, len(registry.Books))
	for _, entry := range registry.Books {
		if entry.Kind == store.KindLibrary {
			continue
		}
		title, titleErr := service.title(entry.ID)
		if titleErr != nil {
			return nil, titleErr
		}
		entries = append(entries, render.IndexEntry{Title: title, Href: indexHref(entry.Path)})
	}
	return entries, nil
}

// IndexEntries is the render catalog hook wired in cmd/pecia.
func IndexEntries(env *cli.Env) ([]render.IndexEntry, *cli.Error) {
	service, err := FromEnv(env)
	if err != nil {
		return nil, err
	}
	return service.Index()
}

func indexHref(outputPath string) string {
	return path.Clean(path.Join(outputPath, "README.md"))
}

func (service *Service) title(id model.BookID) (string, *cli.Error) {
	paths, pathsErr := service.Life.Paths(id)
	if pathsErr != nil {
		return "", cli.AsError(pathsErr)
	}
	state, stateErr := service.Life.State(id)
	if stateErr != nil {
		return "", cli.AsError(stateErr)
	}
	if state.Extracted {
		data, readErr := os.ReadFile(filepath.Join(paths.Dir, lifecycle.ManifestMember))
		if readErr == nil {
			manifest, decodeErr := model.Decode(data)
			if decodeErr != nil {
				return "", cli.AsError(decodeErr)
			}
			return manifest.Root.Title, nil
		}
		if !errors.Is(readErr, os.ErrNotExist) {
			return "", cli.Fail("library.read-failed", "%v", readErr)
		}
	}
	container, loadErr := store.Load(paths.Container)
	if loadErr != nil {
		return "", cli.AsError(loadErr)
	}
	data, ok := container.Read(lifecycle.ManifestMember)
	if !ok {
		return "", cli.Fail("library.manifest-missing",
			"book %q has no %s member", id, lifecycle.ManifestMember)
	}
	manifest, decodeErr := model.Decode(data)
	if decodeErr != nil {
		return "", cli.AsError(decodeErr)
	}
	return manifest.Root.Title, nil
}
