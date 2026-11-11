package structure

import (
	"strconv"
	"strings"

	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

// applyLibrarySetting writes one library README layout key. The first key materializes the default
// slot order, so a switch never hides the content by accident.
func applyLibrarySetting(settings *model.Settings, key, value string) error {
	layout := ensureLibrary(settings)
	switch key {
	case "library.slots":
		slots, err := parseLibrarySlots(value)
		if err != nil {
			return err
		}
		layout.Slots = slots
	case "library.navigation":
		enabled, err := parseOnOff(key, value)
		if err != nil {
			return err
		}
		layout.Navigation = &enabled
	case "library.badges":
		enabled, err := parseOnOff(key, value)
		if err != nil {
			return err
		}
		layout.Badges = &enabled
	case "library.footer":
		enabled, err := parseOnOff(key, value)
		if err != nil {
			return err
		}
		layout.Footer = &enabled
	case "library.documentIndex":
		enabled, err := parseOnOff(key, value)
		if err != nil {
			return err
		}
		if enabled {
			layout.Slots = insertSlot(layout.Slots, model.SlotDocumentIndex)
		} else {
			layout.Slots = removeSlot(layout.Slots, model.SlotDocumentIndex)
		}
	case "library.toc.heading", "library.documentIndex.heading", "library.note.heading":
		heading, err := parseHeading(key, value)
		if err != nil {
			return err
		}
		metaFor(layout, slotOfKey(key)).Heading = heading
	case "library.toc.title", "library.documentIndex.title", "library.note.title":
		title := strings.TrimSpace(value)
		if title == "" {
			return fail("structure.invalid-value",
				"%s expects a non-empty title; hide the slot instead of clearing its title", key)
		}
		metaFor(layout, slotOfKey(key)).Title = title
	case "library.index.columns":
		columns, err := parseLibraryColumns(value)
		if err != nil {
			return err
		}
		ensureIndex(layout).Columns = columns
	case "library.index.shape":
		shape := strings.TrimSpace(value)
		if shape == "" || !model.ValidIndexShape(shape) {
			return fail("structure.invalid-value",
				"library.index.shape expects auto, list or table, got %q", value)
		}
		ensureIndex(layout).Shape = shape
	default:
		return fail("structure.unknown-setting",
			"unknown library setting %q; use library.slots, library.navigation, library.badges, "+
				"library.footer, library.documentIndex, library.<slot>.heading|title or "+
				"library.index.columns|shape", key)
	}
	return nil
}

// ensureLibrary materializes the default order when the first library key is set.
func ensureLibrary(settings *model.Settings) *model.LibraryLayout {
	if settings.Library == nil {
		settings.Library = &model.LibraryLayout{Slots: model.DefaultSlotOrder()}
	}
	return settings.Library
}

func ensureIndex(layout *model.LibraryLayout) *model.LibraryIndexConfig {
	if layout.Index == nil {
		layout.Index = &model.LibraryIndexConfig{}
	}
	return layout.Index
}

func parseLibrarySlots(value string) ([]string, error) {
	slots := make([]string, 0)
	seen := map[string]bool{}
	for _, raw := range strings.Split(value, ",") {
		slot := strings.TrimSpace(raw)
		if slot == "" {
			continue
		}
		if model.StructuralSlot(slot) {
			return nil, fail("structure.invalid-value", "the slot %q has a fixed position", slot)
		}
		if !model.OrderableSlot(slot) {
			return nil, fail("structure.invalid-value",
				"unknown orderable slot %q; use content, note, status, toc, documentIndex or chapters",
				slot)
		}
		if seen[slot] {
			return nil, fail("structure.invalid-value", "the slot %q is listed twice", slot)
		}
		seen[slot] = true
		slots = append(slots, slot)
	}
	return slots, nil
}

func parseLibraryColumns(value string) ([]string, error) {
	columns := make([]string, 0)
	seen := map[string]bool{}
	hasTitle := false
	for _, raw := range strings.Split(value, ",") {
		column := strings.TrimSpace(raw)
		if column == "" {
			continue
		}
		if !model.ValidIndexColumn(column) {
			return nil, fail("structure.invalid-value",
				"unknown index column %q; use title, description, status or path", column)
		}
		if seen[column] {
			return nil, fail("structure.invalid-value", "the column %q is listed twice", column)
		}
		seen[column] = true
		if column == "title" {
			hasTitle = true
		}
		columns = append(columns, column)
	}
	if !hasTitle {
		return nil, fail("structure.invalid-value", "the index columns must include title")
	}
	return columns, nil
}

func parseHeading(key, value string) (int, error) {
	heading, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || heading < 2 || heading > 6 {
		return 0, fail("structure.invalid-value",
			"%s expects a heading level between 2 and 6, got %q", key, value)
	}
	return heading, nil
}

func slotOfKey(key string) string {
	switch {
	case strings.HasPrefix(key, "library.toc."):
		return model.SlotTOC
	case strings.HasPrefix(key, "library.documentIndex."):
		return model.SlotDocumentIndex
	case strings.HasPrefix(key, "library.note."):
		return model.SlotNote
	}
	return ""
}

func metaFor(layout *model.LibraryLayout, slot string) *model.SlotMeta {
	var meta **model.SlotMeta
	switch slot {
	case model.SlotTOC:
		meta = &layout.Toc
	case model.SlotDocumentIndex:
		meta = &layout.DocumentIndex
	case model.SlotNote:
		meta = &layout.Note
	default:
		return &model.SlotMeta{}
	}
	if *meta == nil {
		*meta = &model.SlotMeta{}
	}
	return *meta
}

func insertSlot(slots []string, slot string) []string {
	if containsSlot(slots, slot) {
		return slots
	}
	position := len(slots)
	for index, candidate := range slots {
		if candidate == model.SlotTOC {
			position = index + 1
		}
	}
	result := make([]string, 0, len(slots)+1)
	result = append(result, slots[:position]...)
	result = append(result, slot)
	result = append(result, slots[position:]...)
	return result
}

func removeSlot(slots []string, slot string) []string {
	result := make([]string, 0, len(slots))
	for _, candidate := range slots {
		if candidate != slot {
			result = append(result, candidate)
		}
	}
	return result
}

func containsSlot(slots []string, slot string) bool {
	for _, candidate := range slots {
		if candidate == slot {
			return true
		}
	}
	return false
}
