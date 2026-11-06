package model

// Status is one state of the tool-owned editorial vocabulary. The states are ordered from the
// highest state to the lowest. Color and Description feed the generated shields.io badge and the
// legend, so authors never write the status markup.
type Status struct {
	ID          string
	Label       string
	Color       string
	Description string
}

// Statuses is the fixed editorial vocabulary. The tool owns it; a book only switches it on and
// assigns states to nodes and blocks.
var Statuses = []Status{
	{
		ID: "final", Label: "Final", Color: "brightgreen",
		Description: "Content is settled; only errata and clarifications remain.",
	},
	{
		ID: "review", Label: "In Review", Color: "yellow",
		Description: "Content is complete and under review; small changes are expected.",
	},
	{
		ID: "wip", Label: "Work in Progress", Color: "orange",
		Description: "The direction is set, details are still moving.",
	},
	{
		ID: "draft", Label: "Draft", Color: "lightgrey",
		Description: "An outline and a direction; the content is not complete.",
	},
}

// FindStatus returns the state for an id.
func FindStatus(id string) (Status, bool) {
	for _, status := range Statuses {
		if status.ID == id {
			return status, true
		}
	}
	return Status{}, false
}

// StatusConfig is the per-book switch for the status system.
type StatusConfig struct {
	Enabled bool `json:"enabled"`
}

// StatusEnabled reports whether the book renders the status system. Assignments are ignored while
// the system is switched off, so disabling it is never destructive.
func (manifest *Manifest) StatusEnabled() bool {
	return manifest != nil && manifest.Status != nil && manifest.Status.Enabled
}
