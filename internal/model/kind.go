package model

// Kind is the structural or index kind of a node. The kind is bound to a depth: exactly three
// structural levels exist, plus link entries that live among the chapters.
type Kind string

const (
	// KindTitle is the book title level. Exactly one exists, at the root.
	KindTitle Kind = "title"
	// KindChapter is a chapter under the title.
	KindChapter Kind = "chapter"
	// KindSubchapter is a subchapter under a chapter.
	KindSubchapter Kind = "subchapter"
	// KindLink is an index entry pointing at another book, node or block.
	KindLink Kind = "link"
)

// Valid reports whether the kind is known.
func (kind Kind) Valid() bool {
	switch kind {
	case KindTitle, KindChapter, KindSubchapter, KindLink:
		return true
	}
	return false
}

// Structural reports whether the kind is a structural level with blocks and possibly children.
func (kind Kind) Structural() bool {
	return kind == KindTitle || kind == KindChapter || kind == KindSubchapter
}

// OutputMode selects how a book is rendered. The mode is a render parameter, never a storage
// decision.
type OutputMode string

const (
	// OutputMonolith renders one README with the index followed by the full text.
	OutputMonolith OutputMode = "monolith"
	// OutputChapter writes one file per chapter.
	OutputChapter OutputMode = "chapter"
	// OutputPart writes one file per part.
	OutputPart OutputMode = "part"
)

// Valid reports whether the mode is known.
func (mode OutputMode) Valid() bool {
	switch mode {
	case OutputMonolith, OutputChapter, OutputPart:
		return true
	}
	return false
}
