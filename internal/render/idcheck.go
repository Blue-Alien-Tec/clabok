package render

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

type duplicateID struct {
	ID    string   `json:"id"`
	Books []string `json:"books"`
}

type duplicateIDPayload struct {
	Duplicates []duplicateID `json:"duplicates"`
}

func (payload duplicateIDPayload) Text() string {
	var builder strings.Builder
	for _, duplicate := range payload.Duplicates {
		fmt.Fprintf(&builder, "id %s: %s\n", duplicate.ID, strings.Join(duplicate.Books, ", "))
	}
	return builder.String()
}

// checkLibraryIDs rejects an owned ID that appears in more than one registered book. It is a
// read-only integrity check: IDs are immutable, so a duplicate is corruption to report, never to
// rename.
func checkLibraryIDs(service *Service, registry *store.Registry) *cli.Error {
	owners := make(map[model.ID]map[model.BookID]struct{})
	for _, entry := range registry.Books {
		source, err := service.sources.LoadManifest(entry.ID)
		if err != nil {
			return cli.AsError(err)
		}
		for _, id := range source.Manifest.OwnedIDs() {
			if id.IsZero() {
				continue
			}
			if owners[id] == nil {
				owners[id] = make(map[model.BookID]struct{})
			}
			owners[id][entry.ID] = struct{}{}
		}
	}
	duplicates := make([]duplicateID, 0)
	for id, books := range owners {
		if len(books) < 2 {
			continue
		}
		names := make([]string, 0, len(books))
		for book := range books {
			names = append(names, string(book))
		}
		sort.Strings(names)
		duplicates = append(duplicates, duplicateID{ID: id.String(), Books: names})
	}
	if len(duplicates) == 0 {
		return nil
	}
	sort.Slice(duplicates, func(left, right int) bool {
		leftID, _ := model.ParseID(duplicates[left].ID)
		rightID, _ := model.ParseID(duplicates[right].ID)
		return leftID < rightID
	})
	return cli.Fail("library.id-duplicate",
		"%d ID(s) are owned by more than one book", len(duplicates)).
		WithHint("IDs are immutable; restore the affected manifest instead of renaming an ID").
		WithDetails(duplicateIDPayload{Duplicates: duplicates})
}
