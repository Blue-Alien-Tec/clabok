package search

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// Mode is a text match mode.
type Mode string

const (
	// ModeLiteral matches an exact substring.
	ModeLiteral Mode = "literal"
	// ModeGlob matches `*` and `?` against the whole line or title.
	ModeGlob Mode = "glob"
	// ModeRegex matches a Go regular expression.
	ModeRegex Mode = "regex"
	// ModeFuzzy matches word by word within a bounded edit distance.
	ModeFuzzy Mode = "fuzzy"
)

// ParseMode reports whether name is a supported match mode.
func ParseMode(name string) (Mode, bool) {
	mode := Mode(name)
	switch mode {
	case ModeLiteral, ModeGlob, ModeRegex:
		return mode, true
	}
	return "", false
}

// ParseQueryMode reports whether name is a supported per-query mode. It adds fuzzy to the global
// modes.
func ParseQueryMode(name string) (Mode, bool) {
	if Mode(name) == ModeFuzzy {
		return ModeFuzzy, true
	}
	return ParseMode(name)
}

// maxRegexBytes bounds a compiled pattern. A longer query is rejected instead of compiled.
const maxRegexBytes = 4096

// Span is one match inside a line. Offset and Length are bytes in the line, Column is the 1-based
// rune index of the first byte. Query is the zero-based query index, Distance the fuzzy edit
// distance (0 outside fuzzy matching).
type Span struct {
	Column   int    `json:"column"`
	Offset   int    `json:"offset"`
	Length   int    `json:"length"`
	Text     string `json:"text"`
	Query    int    `json:"query"`
	Distance int    `json:"distance"`
}

// MatcherOptions are the inputs of Compile.
type MatcherOptions struct {
	Mode       Mode
	IgnoreCase bool
	Word       bool
	Fuzzy      int
}

// Matcher matches one query against one line. A matcher is immutable and safe for concurrent use.
type Matcher struct {
	mode       Mode
	word       bool
	ignoreCase bool
	query      string
	expression *regexp.Regexp
	fuzzy      *fuzzyMatcher
}

// Compile prepares one query. Word boundaries are literal-only; fuzzy matching needs literal mode
// and a distance of 1 to 3. Case folding for literal matching is strings.ToLower, so a line whose
// fold changes the byte length reports shifted offsets.
func Compile(query string, options MatcherOptions) (*Matcher, error) {
	if options.Fuzzy < 0 || options.Fuzzy > 3 {
		return nil, fail("search.invalid-fuzzy", "--fuzzy accepts 1 to 3")
	}
	if options.Fuzzy > 0 && options.Mode != ModeLiteral && options.Mode != ModeFuzzy {
		return nil, fail("search.fuzzy-mode", "--fuzzy needs literal matching")
	}
	if options.Word && (options.Mode == ModeFuzzy || options.Fuzzy > 0) {
		return nil, fail("search.fuzzy-word", "--word cannot be combined with --fuzzy")
	}
	if options.Word && options.Mode != ModeLiteral {
		return nil, fail("search.word-mode", "--word is only valid with --mode literal")
	}
	matcher := &Matcher{
		mode:       options.Mode,
		word:       options.Word,
		ignoreCase: options.IgnoreCase,
		query:      query,
	}
	switch options.Mode {
	case ModeLiteral:
		if options.Fuzzy > 0 {
			fuzzy, err := newFuzzyMatcher(query, options.IgnoreCase, options.Fuzzy)
			if err != nil {
				return nil, err
			}
			matcher.fuzzy = fuzzy
			matcher.mode = ModeFuzzy
			return matcher, nil
		}
		if options.IgnoreCase {
			matcher.query = strings.ToLower(query)
		}
		return matcher, nil
	case ModeFuzzy:
		distance := options.Fuzzy
		if distance == 0 {
			distance = 1
		}
		fuzzy, err := newFuzzyMatcher(query, options.IgnoreCase, distance)
		if err != nil {
			return nil, err
		}
		matcher.fuzzy = fuzzy
		return matcher, nil
	case ModeGlob:
		expression, err := regexp.Compile(globExpression(query, options.IgnoreCase))
		if err != nil {
			return nil, fail("search.invalid-glob", "invalid glob %q: %v", query, err)
		}
		matcher.expression = expression
		return matcher, nil
	case ModeRegex:
		if len(query) > maxRegexBytes {
			return nil, fail("search.invalid-regex", "the pattern exceeds %d bytes", maxRegexBytes)
		}
		pattern := query
		if options.IgnoreCase {
			pattern = "(?i)" + pattern
		}
		expression, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fail("search.invalid-regex", "invalid regular expression %q: %v", query, err)
		}
		matcher.expression = expression
		return matcher, nil
	}
	return nil, fail("search.invalid-mode", "unknown match mode %q", options.Mode)
}

// Mode returns the match mode.
func (matcher *Matcher) Mode() Mode { return matcher.mode }

// Match returns every occurrence of the query in text. An empty result means no match.
func (matcher *Matcher) Match(text string) []Span {
	if matcher.fuzzy != nil {
		return matcher.fuzzy.Match(text)
	}
	switch matcher.mode {
	case ModeLiteral:
		return matcher.literalSpans(text)
	case ModeGlob, ModeRegex:
		return matcher.expressionSpans(text)
	}
	return nil
}

func (matcher *Matcher) literalSpans(text string) []Span {
	if matcher.query == "" {
		return nil
	}
	search := text
	if matcher.ignoreCase {
		search = strings.ToLower(text)
	}
	spans := make([]Span, 0)
	for index := 0; index+len(matcher.query) <= len(search); {
		end := index + len(matcher.query)
		if search[index:end] == matcher.query && (!matcher.word || wordBoundary(search, index, end)) {
			spans = append(spans, makeSpan(text, index, end))
			index = end
			continue
		}
		_, size := utf8.DecodeRuneInString(search[index:])
		if size == 0 {
			size = 1
		}
		index += size
	}
	return spans
}

func (matcher *Matcher) expressionSpans(text string) []Span {
	locations := matcher.expression.FindAllStringIndex(text, -1)
	spans := make([]Span, 0, len(locations))
	var empty *Span
	for _, location := range locations {
		if location[0] == location[1] {
			if empty == nil {
				span := makeSpan(text, location[0], location[1])
				empty = &span
			}
			continue
		}
		spans = append(spans, makeSpan(text, location[0], location[1]))
	}
	if len(spans) == 0 && empty != nil {
		return []Span{*empty}
	}
	return spans
}

func makeSpan(text string, start, end int) Span {
	return Span{
		Column: utf8.RuneCountInString(text[:start]) + 1,
		Offset: start,
		Length: end - start,
		Text:   text[start:end],
	}
}

// globExpression translates a glob into an anchored regular expression: `*` matches any sequence,
// `?` matches one rune, and `\` escapes the next rune.
func globExpression(pattern string, ignoreCase bool) string {
	var builder strings.Builder
	if ignoreCase {
		builder.WriteString("(?i)")
	}
	builder.WriteString("^")
	for index := 0; index < len(pattern); {
		character, size := utf8.DecodeRuneInString(pattern[index:])
		index += size
		switch character {
		case '*':
			builder.WriteString(".*")
		case '?':
			builder.WriteString(".")
		case '\\':
			if index >= len(pattern) {
				builder.WriteString(`\\`)
				continue
			}
			escaped, escapedSize := utf8.DecodeRuneInString(pattern[index:])
			index += escapedSize
			builder.WriteString(regexp.QuoteMeta(string(escaped)))
		default:
			builder.WriteString(regexp.QuoteMeta(string(character)))
		}
	}
	builder.WriteString("$")
	return builder.String()
}
