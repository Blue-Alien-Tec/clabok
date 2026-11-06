package library

import (
	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/render"
)

// SyncBook renders one registered book with the full library wiring (navigation, Document Index and
// cross-book resolvers).
func (service *Service) SyncBook(id model.BookID, options render.SyncOptions) (render.SyncResult, *cli.Error) {
	registry, registryErr := service.loadRegistry()
	if registryErr != nil {
		return render.SyncResult{}, registryErr
	}
	entry, exists := registry.Find(id)
	if !exists {
		return render.SyncResult{}, cli.Reject("library.unknown-book", "book %s is not registered", id)
	}
	renderService, renderErr := service.preparedRender(registry, entry)
	if renderErr != nil {
		return render.SyncResult{}, renderErr
	}
	result, syncErr := renderService.Sync(id, service.target(entry.Path), options)
	if syncErr != nil {
		return render.SyncResult{}, cli.AsError(syncErr)
	}
	return result, nil
}

// CheckBook runs the full render check of one registered book.
func (service *Service) CheckBook(id model.BookID) ([]render.Finding, *cli.Error) {
	registry, registryErr := service.loadRegistry()
	if registryErr != nil {
		return nil, registryErr
	}
	entry, exists := registry.Find(id)
	if !exists {
		return nil, cli.Reject("library.unknown-book", "book %s is not registered", id)
	}
	renderService, renderErr := service.preparedRender(registry, entry)
	if renderErr != nil {
		return nil, renderErr
	}
	findings, checkErr := renderService.Check(id)
	if checkErr != nil {
		return nil, cli.AsError(checkErr)
	}
	return findings, nil
}
