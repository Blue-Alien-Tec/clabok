package render

import "github.com/Blue-Alien-Tec/clabok/internal/model"

// CrossLink is a resolved target in another book.
type CrossLink struct {
	Title string
	Href  string
}

// CrossLinkFunc resolves a globally unique ID in another book. The href is already relative to the
// current book's output directory.
type CrossLinkFunc func(model.ID) (CrossLink, bool)

// CrossBookFunc resolves another registered book to its README href, relative to the current book.
type CrossBookFunc func(model.BookID) (string, bool)
