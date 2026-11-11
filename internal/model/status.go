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

// BuiltinDescription returns the fallback legend text of a built-in level.
func BuiltinDescription(id string) string {
	if status, ok := FindStatus(id); ok {
		return status.Description
	}
	return ""
}

// StatusLevel is one configured level of a book. The ID is the stable assignment key; label and
// color are presentation; the meaning is prose and lives in a block.
type StatusLevel struct {
	ID               string `json:"id"`
	Label            string `json:"label"`
	Color            string `json:"color,omitempty"`
	DescriptionBlock ID     `json:"descriptionBlock,omitempty"`
}

// StatusLevelCap bounds the configured list. The legend is a readable table, and a book has no use
// for more editorial states than eight.
const StatusLevelCap = 8

// StatusConfig is the per-book switch and level list of the status system. An empty Levels list
// means the built-in fallback; a non-empty list is authoritative.
type StatusConfig struct {
	Enabled bool          `json:"enabled"`
	Levels  []StatusLevel `json:"levels,omitempty"`
}

// StatusEnabled reports whether the book renders the status system. Assignments are ignored while
// the system is switched off, so disabling it is never destructive.
func (manifest *Manifest) StatusEnabled() bool {
	return manifest != nil && manifest.Status != nil && manifest.Status.Enabled
}

// StatusFallback reports whether the built-in levels are in effect.
func (manifest *Manifest) StatusFallback() bool {
	return manifest == nil || manifest.Status == nil || len(manifest.Status.Levels) == 0
}

// StatusLevels returns the effective level list: the configured levels, or the built-in four mapped
// into configured levels without description blocks.
func (manifest *Manifest) StatusLevels() []StatusLevel {
	if manifest != nil && manifest.Status != nil && len(manifest.Status.Levels) > 0 {
		return append([]StatusLevel(nil), manifest.Status.Levels...)
	}
	levels := make([]StatusLevel, 0, len(Statuses))
	for _, status := range Statuses {
		levels = append(levels, StatusLevel{ID: status.ID, Label: status.Label, Color: status.Color})
	}
	return levels
}

// StatusLevel returns one effective level by id.
func (manifest *Manifest) StatusLevel(id string) (StatusLevel, bool) {
	for _, level := range manifest.StatusLevels() {
		if level.ID == id {
			return level, true
		}
	}
	return StatusLevel{}, false
}
