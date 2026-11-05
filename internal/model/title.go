package model

import (
	"regexp"
	"strings"
)

var (
	romanPrefix  = regexp.MustCompile(`^[IVXLCDM]+\.\s+`)
	arabicPrefix = regexp.MustCompile(`^(?:[0-9]+(?:\.[0-9]+)+\.?|[0-9]+\.)\s+`)
)

// NormalizeTitle strips a leading numeric prefix. Titles are stored without numbers; numbering is
// applied at render time.
//
// Stripped: "1. Ziel", "1.2 Ziel", "1.2.3 Ziel", "IV. Kapitel".
// Kept: "2026 - Year of X" (no dot), "3D Printing" (no separator), "1.Ziel" (no space).
func NormalizeTitle(title string) string {
	trimmed := strings.TrimSpace(title)
	stripped := romanPrefix.ReplaceAllString(trimmed, "")
	stripped = arabicPrefix.ReplaceAllString(stripped, "")
	stripped = strings.TrimSpace(stripped)
	if stripped == "" {
		return trimmed
	}
	return stripped
}
