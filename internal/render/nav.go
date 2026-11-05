package render

import (
	"strings"

	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

// Navigation describes the current book's place in the library.
type Navigation struct {
	// IsLibrary marks the library book. Its README navigates to the Document Index.
	IsLibrary bool
	// LibraryHref is the href of the library README relative to the current book, or "" when no
	// library is registered.
	LibraryHref string
}

const (
	navLibrary      = "← Library"
	navFirst        = "First document →"
	navDocumentIx   = "Document Index"
	navSeparator    = " · "
	navIndex        = "[Index](README.md)"
	navDocumentLink = "[Document Index](#document-index)"
)

func navJoin(parts []string) string {
	return strings.Join(parts, navSeparator)
}

// readmeNav returns the top and bottom navigation of a book README. The library jumps to its
// Document Index; every other book jumps to the library and, in split modes, to its first document.
func readmeNav(nav Navigation, index []IndexEntry, first string) (top, bottom string) {
	if nav.IsLibrary {
		if len(index) == 0 {
			return "", ""
		}
		return navDocumentLink, navDocumentLink
	}
	topParts := make([]string, 0, 1)
	if nav.LibraryHref != "" {
		topParts = append(topParts, "["+navLibrary+"]("+nav.LibraryHref+")")
	}
	bottomParts := append([]string(nil), topParts...)
	if first != "" {
		bottomParts = append(bottomParts, "["+navFirst+"]("+first+")")
	}
	return navJoin(topParts), navJoin(bottomParts)
}

// chapterNav renders the per-file navigation. Previous and Next are omitted at the boundaries.
func chapterNav(previous, next string) string {
	parts := make([]string, 0, 3)
	if previous != "" {
		parts = append(parts, "[← Previous]("+previous+")")
	}
	parts = append(parts, navIndex)
	if next != "" {
		parts = append(parts, "[Next →]("+next+")")
	}
	return navJoin(parts)
}

// firstDocument returns the first content file of a split mode, or "".
func firstDocument(manifest *model.Manifest, layout map[model.ID]LinkTarget, parts []string, partFiles map[string]string) string {
	if manifest.Settings.Output == model.OutputPart {
		if len(parts) > 0 {
			return partFiles[parts[0]]
		}
		return ""
	}
	for _, chapter := range manifest.Root.Children {
		if chapter.Kind != model.KindChapter {
			continue
		}
		if target, ok := layout[chapter.ID]; ok && target.File != "" {
			return target.File
		}
	}
	return ""
}
