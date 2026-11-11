package model

import "strings"

// Orderable README slots. They are freely orderable; an unlisted slot is hidden.
const (
	SlotContent       = "content"
	SlotNote          = "note"
	SlotStatus        = "status"
	SlotTOC           = "toc"
	SlotDocumentIndex = "documentIndex"
	SlotChapters      = "chapters"
)

// Structural README positions are fixed by the renderer and cannot be ordered.
const (
	StructuralNavigation    = "nav"
	StructuralNavigationEnd = "navEnd"
	StructuralBadges        = "badges"
	StructuralTitle         = "title"
	StructuralFooter        = "footer"
)

// IndexShape values of the library book index.
const (
	IndexShapeAuto  = "auto"
	IndexShapeList  = "list"
	IndexShapeTable = "table"
)

// orderableSlots is the default order and the set of addressable slots.
var orderableSlots = []string{
	SlotContent, SlotStatus, SlotTOC, SlotDocumentIndex, SlotChapters,
}

// SlotMeta is the heading level and optional heading text of one slot.
type SlotMeta struct {
	Heading int    `json:"heading,omitempty"`
	Title   string `json:"title,omitempty"`
}

// LibraryIndexConfig is the projection of the library book index.
type LibraryIndexConfig struct {
	Columns []string `json:"columns,omitempty"`
	Shape   string   `json:"shape,omitempty"`
}

// LibraryLayout orders the README slots of one book. A nil layout is the default output.
type LibraryLayout struct {
	Slots         []string            `json:"slots,omitempty"`
	Navigation    *bool               `json:"navigation,omitempty"`
	Badges        *bool               `json:"badges,omitempty"`
	Footer        *bool               `json:"footer,omitempty"`
	Toc           *SlotMeta           `json:"toc,omitempty"`
	DocumentIndex *SlotMeta           `json:"documentIndex,omitempty"`
	Note          *SlotMeta           `json:"note,omitempty"`
	Index         *LibraryIndexConfig `json:"index,omitempty"`
}

// OrderableSlot reports whether a slot may appear in the order list.
func OrderableSlot(slot string) bool {
	for _, candidate := range orderableSlots {
		if candidate == slot {
			return true
		}
	}
	return false
}

// StructuralSlot reports a slot whose position is fixed.
func StructuralSlot(slot string) bool {
	switch slot {
	case StructuralNavigation, StructuralNavigationEnd, StructuralBadges, StructuralTitle,
		StructuralFooter:
		return true
	}
	return false
}

// ValidIndexShape reports a known index shape.
func ValidIndexShape(shape string) bool {
	switch shape {
	case "", IndexShapeAuto, IndexShapeList, IndexShapeTable:
		return true
	}
	return false
}

// ValidIndexColumn reports a known index column.
func ValidIndexColumn(column string) bool {
	switch column {
	case "title", "description", "status", "path":
		return true
	}
	return false
}

// DefaultSlotOrder returns the order that renders today's README.
func DefaultSlotOrder() []string { return append([]string(nil), orderableSlots...) }

// SlotsOrDefault returns the configured order or the default order. A nil layout yields the
// default; a layout with an explicit empty list hides every orderable slot.
func (layout *LibraryLayout) SlotsOrDefault() []string {
	if layout == nil {
		return DefaultSlotOrder()
	}
	return append([]string(nil), layout.Slots...)
}

// Shows reports whether an orderable slot is rendered.
func (layout *LibraryLayout) Shows(slot string) bool {
	for _, candidate := range layout.SlotsOrDefault() {
		if candidate == slot {
			return true
		}
	}
	return false
}

// NavigationEnabled reports the navigation switch; the default is on.
func (layout *LibraryLayout) NavigationEnabled() bool {
	if layout == nil {
		return true
	}
	return boolOrDefault(layout.Navigation)
}

// BadgesEnabled reports the badge switch; the default is on.
func (layout *LibraryLayout) BadgesEnabled() bool {
	if layout == nil {
		return true
	}
	return boolOrDefault(layout.Badges)
}

// FooterEnabled reports the footer switch; the default is on.
func (layout *LibraryLayout) FooterEnabled() bool {
	if layout == nil {
		return true
	}
	return boolOrDefault(layout.Footer)
}

// SlotHeading returns the heading level of a slot, defaulting to level 2.
func (layout *LibraryLayout) SlotHeading(slot string) int {
	if meta := layout.meta(slot); meta != nil && meta.Heading > 0 {
		return meta.Heading
	}
	return 2
}

// SlotTitle returns the heading text of a slot or the fallback.
func (layout *LibraryLayout) SlotTitle(slot, fallback string) string {
	if meta := layout.meta(slot); meta != nil && strings.TrimSpace(meta.Title) != "" {
		return meta.Title
	}
	return fallback
}

// IndexConfig returns the book-index projection settings, or nil for the derived default.
func (layout *LibraryLayout) IndexConfig() *LibraryIndexConfig {
	if layout == nil {
		return nil
	}
	return layout.Index
}

func (layout *LibraryLayout) meta(slot string) *SlotMeta {
	if layout == nil {
		return nil
	}
	switch slot {
	case SlotTOC:
		return layout.Toc
	case SlotDocumentIndex:
		return layout.DocumentIndex
	case SlotNote:
		return layout.Note
	}
	return nil
}

func boolOrDefault(value *bool) bool {
	if value == nil {
		return true
	}
	return *value
}
