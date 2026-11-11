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
	// Books are the other registered books, in library order, with hrefs relative to the current
	// book's README.
	Books []BookLink
}

// BookLink is one direct sibling-book link in the navigation strip.
type BookLink struct {
	Title string
	Href  string
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

// readmeNav returns the top and bottom navigation of a book README. Every registered book appears
// as a direct link, in library order; the current book is not linked to itself. The library jumps to
// its Document Index.
func readmeNav(nav Navigation, index []IndexEntry, first string, showDocumentIndex bool) (top, bottom string) {
	books := make([]string, 0, len(nav.Books))
	for _, book := range nav.Books {
		if book.Title == "" || book.Href == "" {
			continue
		}
		books = append(books, "["+plainTitle(book.Title)+"]("+book.Href+")")
	}
	if nav.IsLibrary {
		parts := append([]string(nil), books...)
		if len(index) > 0 && showDocumentIndex {
			parts = append(parts, navDocumentLink)
		}
		if len(parts) == 0 {
			return "", ""
		}
		text := navJoin(parts)
		return text, text
	}
	topParts := make([]string, 0, 1+len(books))
	if nav.LibraryHref != "" {
		topParts = append(topParts, "["+navLibrary+"]("+nav.LibraryHref+")")
	}
	topParts = append(topParts, books...)
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

// documentIndexShown reports whether the README renders the document index slot. The navigation
// must not link a section the book hides.
func documentIndexShown(manifest *model.Manifest) bool {
	return manifest.Settings.Library.Shows(model.SlotDocumentIndex)
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
