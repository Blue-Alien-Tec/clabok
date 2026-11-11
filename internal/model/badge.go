package model

import (
	"strconv"
	"strings"
)

// Badge is one declared README badge. Source names the detector; an explicit message overrides it.
type Badge struct {
	ID           string `json:"id"`
	Source       string `json:"source,omitempty"`
	Label        string `json:"label,omitempty"`
	Message      string `json:"message,omitempty"`
	Link         string `json:"link,omitempty"`
	Style        string `json:"style,omitempty"`
	Logo         string `json:"logo,omitempty"`
	LogoColor    string `json:"logoColor,omitempty"`
	LabelColor   string `json:"labelColor,omitempty"`
	Color        string `json:"color,omitempty"`
	Alt          string `json:"alt,omitempty"`
	CacheSeconds int    `json:"cacheSeconds,omitempty"`
	Required     bool   `json:"required,omitempty"`
	Order        int    `json:"order,omitempty"`
	Group        string `json:"group,omitempty"`
}

// BadgeStyles are the shields.io styles a badge may declare.
var BadgeStyles = []string{"flat", "flat-square", "plastic", "for-the-badge", "social"}

// badgeColors are the named shields colors. A bare three- or six-digit hex value is accepted too.
var badgeColors = map[string]bool{
	"brightgreen": true, "green": true, "yellowgreen": true, "yellow": true, "orange": true,
	"red": true, "blue": true, "lightgrey": true, "blueviolet": true, "success": true,
	"important": true, "critical": true, "informational": true, "inactive": true,
	"ff69b4": true,
}

// ValidBadgeStyle reports a known shields style; the empty value is the default.
func ValidBadgeStyle(style string) bool {
	if style == "" {
		return true
	}
	for _, candidate := range BadgeStyles {
		if candidate == style {
			return true
		}
	}
	return false
}

// ValidBadgeColor reports a named shields color or a three- or six-digit hex value.
func ValidBadgeColor(color string) bool {
	if color == "" {
		return true
	}
	lower := strings.ToLower(color)
	if badgeColors[lower] {
		return true
	}
	if len(lower) != 3 && len(lower) != 6 {
		return false
	}
	_, err := strconv.ParseUint(lower, 16, 32)
	return err == nil
}

// ValidBadgeLogo reports a simple-icons slug. A URL or a local path is never a badge logo.
func ValidBadgeLogo(logo string) bool {
	if logo == "" {
		return true
	}
	if len(logo) > 32 {
		return false
	}
	for _, runeValue := range logo {
		switch {
		case runeValue >= 'a' && runeValue <= 'z':
		case runeValue >= 'A' && runeValue <= 'Z':
		case runeValue >= '0' && runeValue <= '9':
		case runeValue == '.', runeValue == '_', runeValue == '-':
		default:
			return false
		}
	}
	return true
}
