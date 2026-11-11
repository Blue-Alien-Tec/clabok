package model

import "strings"

// ValidateLanguageTag reports whether a language id has the shape of a BCP-47 tag: a two- or
// three-letter primary subtag followed by optional alphanumeric subtags. Server support is a
// command-time question, not a model concern.
func ValidateLanguageTag(tag string) bool {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return false
	}
	parts := strings.Split(tag, "-")
	if len(parts[0]) < 2 || len(parts[0]) > 3 {
		return false
	}
	if !letters(parts[0]) {
		return false
	}
	for _, part := range parts[1:] {
		if len(part) < 2 || len(part) > 8 || !alphanumeric(part) {
			return false
		}
	}
	return true
}

func letters(text string) bool {
	for _, runeValue := range text {
		if (runeValue < 'a' || runeValue > 'z') && (runeValue < 'A' || runeValue > 'Z') {
			return false
		}
	}
	return true
}

func alphanumeric(text string) bool {
	for _, runeValue := range text {
		letter := (runeValue >= 'a' && runeValue <= 'z') || (runeValue >= 'A' && runeValue <= 'Z')
		digit := runeValue >= '0' && runeValue <= '9'
		if !letter && !digit {
			return false
		}
	}
	return true
}
