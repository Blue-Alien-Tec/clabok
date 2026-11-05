package model

// Block is one body with its ID. Blocks have no placement data: the ordered lists on nodes and
// regions own placement.
type Block struct {
	ID   ID
	Body string
}

// Book couples a manifest with the body text. Bodies are absent from the manifest by design; the
// store keeps them as separate members.
type Book struct {
	Manifest *Manifest
	Bodies   map[ID]string
}

// NewBook returns a book with an empty body map.
func NewBook(manifest *Manifest) *Book {
	return &Book{Manifest: manifest, Bodies: map[ID]string{}}
}

// Body returns the body text of a block.
func (book *Book) Body(id ID) (string, bool) {
	body, ok := book.Bodies[id]
	return body, ok
}

// SetBody writes the body text of a block.
func (book *Book) SetBody(id ID, body string) {
	if book.Bodies == nil {
		book.Bodies = map[ID]string{}
	}
	book.Bodies[id] = body
}

// Block returns the block with its body.
func (book *Book) Block(id ID) (Block, bool) {
	body, ok := book.Bodies[id]
	if !ok {
		return Block{}, false
	}
	return Block{ID: id, Body: body}, true
}

// Clone returns a deep copy of manifest and bodies.
func (book *Book) Clone() *Book {
	if book == nil {
		return nil
	}
	clone := &Book{Manifest: book.Manifest.Clone(), Bodies: make(map[ID]string, len(book.Bodies))}
	for id, body := range book.Bodies {
		clone.Bodies[id] = body
	}
	return clone
}
