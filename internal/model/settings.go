package model

// DefaultIndexDepth is the index depth used when the manifest does not set one.
const DefaultIndexDepth = 2

// Numbering is the per-book numbering switch. Numbers exist only in the rendered result.
type Numbering struct {
	Enabled bool `json:"enabled"`
}

// TOC is the per-file table-of-contents switch.
type TOC struct {
	Enabled bool `json:"enabled"`
}

// Settings are the per-book render settings. They live with the book; the library holds defaults
// and policy, never per-book values.
type Settings struct {
	Numbering  Numbering  `json:"numbering"`
	TOC        TOC        `json:"toc"`
	IndexDepth int        `json:"indexDepth"`
	Output     OutputMode `json:"output"`
}

// DefaultSettings returns the settings of a new book.
func DefaultSettings() Settings {
	return Settings{IndexDepth: DefaultIndexDepth, Output: OutputMonolith}
}

// normalize fills the two settings whose zero value cannot mean "unset".
func (settings *Settings) normalize() {
	if settings.IndexDepth <= 0 {
		settings.IndexDepth = DefaultIndexDepth
	}
	if settings.Output == "" {
		settings.Output = OutputMonolith
	}
}
