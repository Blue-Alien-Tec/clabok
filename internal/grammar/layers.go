package grammar

import (
	"sort"
	"strings"

	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

// StoreLanguages returns the distinct effective languages of the registered books. A store without
// a declared language falls back to the configured default. The set is derived, never stored.
func StoreLanguages(life *lifecycle.Lifecycle, layout store.Layout, fallback string) ([]string, error) {
	registry, err := store.LoadRegistry(layout.Registry())
	if err != nil {
		return nil, err
	}
	seen := map[string]string{}
	for _, entry := range registry.Books {
		settings, settingsErr := settingsOf(life, layout, entry.ID)
		if settingsErr != nil {
			return nil, settingsErr
		}
		language := Resolve(settings, fallback)
		key := strings.ToLower(language)
		if _, ok := seen[key]; !ok {
			seen[key] = language
		}
	}
	if len(seen) == 0 {
		seen[strings.ToLower(fallback)] = fallback
	}
	languages := make([]string, 0, len(seen))
	for _, language := range seen {
		languages = append(languages, language)
	}
	sort.Slice(languages, func(left, right int) bool {
		return strings.ToLower(languages[left]) < strings.ToLower(languages[right])
	})
	return languages, nil
}

// settingsOf reads an open book from the extract and a closed book from its container.
func settingsOf(life *lifecycle.Lifecycle, layout store.Layout, id model.BookID) (model.Settings, error) {
	if settings, err := BookSettings(life, id); err == nil {
		return settings, nil
	}
	paths, err := layout.Book(id)
	if err != nil {
		return model.Settings{}, err
	}
	container, err := store.Load(paths.Container)
	if err != nil {
		return model.Settings{}, err
	}
	data, ok := container.Read(lifecycle.ManifestMember)
	if !ok {
		return model.Settings{}, &ServerError{Code: "grammar.manifest-missing",
			Message: "the book container has no manifest"}
	}
	manifest, err := model.Decode(data)
	if err != nil {
		return model.Settings{}, err
	}
	return manifest.Settings, nil
}
