package grammar

import (
	"os"
	"path/filepath"

	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

// BookSettings reads the settings of an open book without extracting it again. Callers resolve the
// effective language with Resolve.
func BookSettings(life *lifecycle.Lifecycle, book model.BookID) (model.Settings, error) {
	paths, err := life.RequireOpen(book)
	if err != nil {
		return model.Settings{}, err
	}
	data, err := os.ReadFile(filepath.Join(paths.Dir, lifecycle.ManifestMember))
	if err != nil {
		return model.Settings{}, err
	}
	manifest, err := model.Decode(data)
	if err != nil {
		return model.Settings{}, err
	}
	return manifest.Settings, nil
}
