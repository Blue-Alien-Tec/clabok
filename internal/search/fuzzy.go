package search

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// maxFuzzyWords bounds the query tokens of one fuzzy query.
const maxFuzzyWords = 16

// word is one token occurrence in a text.
type word struct {
	Start int
	End   int
	Text  string
}

// tokenize returns the maximal runs of letters, digits and underscores.
func tokenize(text string) []word {
	words := make([]word, 0)
	start := -1
	for index, character := range text {
		if isWordRune(character) {
			if start < 0 {
				start = index
			}
			continue
		}
		if start >= 0 {
			words = append(words, word{Start: start, End: index, Text: text[start:index]})
			start = -1
		}
	}
	if start >= 0 {
		words = append(words, word{Start: start, End: len(text), Text: text[start:]})
	}
	return words
}

func isWordRune(character rune) bool {
	return unicode.IsLetter(character) || unicode.IsDigit(character) || character == '_'
}

// fuzzyMatcher matches a query phrase against a line word by word, in order, within a bounded
// Damerau-Levenshtein distance.
type fuzzyMatcher struct {
	tokens     []string
	ignoreCase bool
	distance   int
}

func newFuzzyMatcher(query string, ignoreCase bool, distance int) (*fuzzyMatcher, error) {
	words := tokenize(query)
	if len(words) == 0 {
		return nil, fail("search.invalid-fuzzy", "the query %q contains no word", query)
	}
	if len(words) > maxFuzzyWords {
		return nil, fail("search.too-many-words",
			"the query has %d words; the limit is %d", len(words), maxFuzzyWords)
	}
	tokens := make([]string, len(words))
	for index, entry := range words {
		tokens[index] = entry.Text
		if ignoreCase {
			tokens[index] = strings.ToLower(tokens[index])
		}
	}
	return &fuzzyMatcher{tokens: tokens, ignoreCase: ignoreCase, distance: distance}, nil
}

// fuzzyState is the best prefix match ending at one text word.
type fuzzyState struct {
	total     int
	positions []int
}

// better reports whether left is the preferred solution: smaller total distance, then the earliest
// first word, then the earliest positions.
func better(left, right fuzzyState) bool {
	if right.positions == nil {
		return true
	}
	if left.total != right.total {
		return left.total < right.total
	}
	for index := 0; index < len(left.positions) && index < len(right.positions); index++ {
		if left.positions[index] != right.positions[index] {
			return left.positions[index] < right.positions[index]
		}
	}
	return false
}

// Match returns one span per matched query word.
func (matcher *fuzzyMatcher) Match(text string) []Span {
	words := tokenize(text)
	if len(words) == 0 {
		return nil
	}
	lowered := make([]string, len(words))
	for index, entry := range words {
		lowered[index] = entry.Text
		if matcher.ignoreCase {
			lowered[index] = strings.ToLower(lowered[index])
		}
	}
	var previous []*fuzzyState
	for queryIndex, token := range matcher.tokens {
		current := make([]*fuzzyState, len(words))
		var best *fuzzyState
		for wordIndex := range words {
			if queryIndex > 0 && wordIndex > 0 {
				candidate := previous[wordIndex-1]
				if candidate != nil && (best == nil || better(*candidate, *best)) {
					best = candidate
				}
			}
			distance := editDistance(token, lowered[wordIndex])
			if distance > matcher.distance {
				continue
			}
			if queryIndex > 0 && best == nil {
				continue
			}
			state := fuzzyState{positions: make([]int, 0, queryIndex+1)}
			if queryIndex > 0 {
				state.total = best.total
				state.positions = append(state.positions, best.positions...)
			}
			state.total += distance
			state.positions = append(state.positions, wordIndex)
			current[wordIndex] = &state
		}
		previous = current
	}
	var chosen *fuzzyState
	for _, state := range previous {
		if state != nil && (chosen == nil || better(*state, *chosen)) {
			chosen = state
		}
	}
	if chosen == nil {
		return nil
	}
	spans := make([]Span, 0, len(chosen.positions))
	for queryIndex, position := range chosen.positions {
		entry := words[position]
		distance := editDistance(matcher.tokens[queryIndex], lowered[position])
		spans = append(spans, Span{
			Column:   utf8.RuneCountInString(text[:entry.Start]) + 1,
			Offset:   entry.Start,
			Length:   entry.End - entry.Start,
			Text:     entry.Text,
			Distance: distance,
		})
	}
	return spans
}

// editDistance is the optimal string alignment distance: insert, delete, substitute and adjacent
// transpose each cost one.
func editDistance(left, right string) int {
	first := []rune(left)
	second := []rune(right)
	rows := len(first) + 1
	columns := len(second) + 1
	matrix := make([][]int, rows)
	for row := range matrix {
		matrix[row] = make([]int, columns)
		matrix[row][0] = row
	}
	for column := 0; column < columns; column++ {
		matrix[0][column] = column
	}
	for row := 1; row < rows; row++ {
		for column := 1; column < columns; column++ {
			cost := 1
			if first[row-1] == second[column-1] {
				cost = 0
			}
			value := matrix[row-1][column] + 1
			if candidate := matrix[row][column-1] + 1; candidate < value {
				value = candidate
			}
			if candidate := matrix[row-1][column-1] + cost; candidate < value {
				value = candidate
			}
			if row > 1 && column > 1 && first[row-1] == second[column-2] && first[row-2] == second[column-1] {
				if candidate := matrix[row-2][column-2] + 1; candidate < value {
					value = candidate
				}
			}
			matrix[row][column] = value
		}
	}
	return matrix[rows-1][columns-1]
}
