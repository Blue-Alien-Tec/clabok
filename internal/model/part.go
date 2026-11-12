package model

import "strings"

// PartLimit is the number of parts one book may carry. Raising it is a maintainer decision, never a
// flag: the cap keeps the index grouping readable and the part output mode bounded.
const PartLimit = 10

// partLabels holds the hardcoded default text of each slot. It is a table, not an algorithm, because
// a reader must be able to see the only values that exist.
var partLabels = [...]string{
	"Part I", "Part II", "Part III", "Part IV", "Part V",
	"Part VI", "Part VII", "Part VIII", "Part IX", "Part X",
}

// PartLabel returns the default text of a part slot. Slots start at one and end at PartLimit.
func PartLabel(slot int) (string, bool) {
	if slot < 1 || slot > len(partLabels) {
		return "", false
	}
	return partLabels[slot-1], true
}

// ParsePartSlot reads a decimal slot number. A value that is not made of digits alone is not a slot,
// so a group text that starts with a digit stays a text. The result saturates above PartLimit, which
// lets the caller report an out-of-range slot instead of an unparsable one.
func ParsePartSlot(raw string) (int, bool) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return 0, false
	}
	slot := 0
	for _, character := range text {
		if character < '0' || character > '9' {
			return 0, false
		}
		if slot <= PartLimit {
			slot = slot*10 + int(character-'0')
		}
	}
	return slot, true
}

// Parts returns the distinct part texts of the book in document order. Only the root children carry a
// part; a subchapter inherits its chapter's group by sitting under it and never stores one.
func (manifest *Manifest) Parts() []string {
	if manifest == nil || manifest.Root == nil {
		return nil
	}
	parts := make([]string, 0, len(manifest.Root.Children))
	seen := make(map[string]struct{}, len(manifest.Root.Children))
	for _, node := range manifest.Root.Children {
		if node.Part == "" {
			continue
		}
		if _, exists := seen[node.Part]; exists {
			continue
		}
		seen[node.Part] = struct{}{}
		parts = append(parts, node.Part)
	}
	return parts
}
