package render

import (
	"strings"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

func TestReadmeNavHonorsHiddenDocumentIndex(t *testing.T) {
	manifest := &model.Manifest{Settings: model.DefaultSettings()}
	index := []IndexEntry{{Title: "X", Href: "x.md"}}
	nav := Navigation{IsLibrary: true}

	top, bottom := readmeNav(nav, index, "", documentIndexShown(manifest))
	if !strings.Contains(top, "#document-index") || top != bottom {
		t.Fatalf("default nav %q / %q", top, bottom)
	}

	manifest.Settings.Library = &model.LibraryLayout{
		Slots: []string{model.SlotContent, model.SlotChapters},
	}
	top, bottom = readmeNav(nav, index, "", documentIndexShown(manifest))
	if strings.Contains(top, "#document-index") || strings.Contains(bottom, "#document-index") {
		t.Fatalf("a hidden document index must not be linked: %q / %q", top, bottom)
	}
}
