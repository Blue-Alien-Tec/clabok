// Package prose validates the body prose rules. It is shared by the body commands and the
// read-only render acceptance.
package prose

import "strings"

// Issue is one prose violation.
type Issue struct {
	Line int    `json:"line"`
	Text string `json:"text"`
}

// CheckProse reports #, ## and ### heading lines outside fenced code blocks. #### and deeper are
// text, as are lines indented by four spaces.
func CheckProse(text string) []Issue {
	issues := make([]Issue, 0)
	fence := ""
	for index, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimLeft(line, " \t")
		if fence != "" {
			if strings.HasPrefix(trimmed, fence) {
				fence = ""
			}
			continue
		}
		if strings.HasPrefix(trimmed, "```") {
			fence = "```"
			continue
		}
		if strings.HasPrefix(trimmed, "~~~") {
			fence = "~~~"
			continue
		}
		if level := headingLevel(line); level >= 1 && level <= 3 {
			issues = append(issues, Issue{Line: index + 1, Text: line})
		}
	}
	return issues
}

func headingLevel(line string) int {
	spaces := 0
	for spaces < len(line) && line[spaces] == ' ' {
		spaces++
	}
	if spaces > 3 {
		return 0
	}
	hashes := 0
	for spaces+hashes < len(line) && line[spaces+hashes] == '#' {
		hashes++
	}
	if hashes == 0 {
		return 0
	}
	rest := spaces + hashes
	if rest < len(line) && line[rest] != ' ' && line[rest] != '\t' {
		return 0
	}
	return hashes
}
