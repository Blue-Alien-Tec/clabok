package search

import (
	"reflect"
	"strings"
	"testing"
)

func compileMatcher(t *testing.T, query string, options MatcherOptions) *Matcher {
	t.Helper()
	matcher, err := Compile(query, options)
	if err != nil {
		t.Fatalf("compile %q: %v", query, err)
	}
	return matcher
}

func errorCode(err error) string {
	coded, ok := err.(interface{ ErrorCode() string })
	if !ok {
		return ""
	}
	return coded.ErrorCode()
}

func span(column, offset, length int, text string) Span {
	return Span{Column: column, Offset: offset, Length: length, Text: text}
}

func TestLiteralSpans(t *testing.T) {
	matcher := compileMatcher(t, "aa", MatcherOptions{Mode: ModeLiteral})
	got := matcher.Match("aaaa")
	want := []Span{span(1, 0, 2, "aa"), span(3, 2, 2, "aa")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("spans %+v", got)
	}
	if spans := matcher.Match("a"); len(spans) != 0 {
		t.Fatalf("short line %+v", spans)
	}
}

func TestLiteralIgnoreCase(t *testing.T) {
	matcher := compileMatcher(t, "hello", MatcherOptions{Mode: ModeLiteral, IgnoreCase: true})
	got := matcher.Match("Hello HELLO")
	want := []Span{span(1, 0, 5, "Hello"), span(7, 6, 5, "HELLO")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("spans %+v", got)
	}
}

func TestLiteralWordBoundary(t *testing.T) {
	matcher := compileMatcher(t, "cat", MatcherOptions{Mode: ModeLiteral, Word: true})
	got := matcher.Match("cat concatenate cat")
	want := []Span{span(1, 0, 3, "cat"), span(17, 16, 3, "cat")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("spans %+v", got)
	}
	if spans := matcher.Match("cat-like"); len(spans) != 1 {
		t.Fatalf("hyphen is not a word boundary: %+v", spans)
	}
}

func TestLiteralUnicodeColumns(t *testing.T) {
	matcher := compileMatcher(t, "grüße", MatcherOptions{Mode: ModeLiteral, IgnoreCase: true})
	got := matcher.Match("Grüße grüße")
	want := []Span{span(1, 0, 7, "Grüße"), span(7, 8, 7, "grüße")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("spans %+v", got)
	}
}

func TestGlob(t *testing.T) {
	matcher := compileMatcher(t, "cat*", MatcherOptions{Mode: ModeGlob})
	got := matcher.Match("cats")
	if !reflect.DeepEqual(got, []Span{span(1, 0, 4, "cats")}) {
		t.Fatalf("spans %+v", got)
	}
	if spans := matcher.Match("a cat"); len(spans) != 0 {
		t.Fatalf("a glob is anchored: %+v", spans)
	}
	question := compileMatcher(t, "?at", MatcherOptions{Mode: ModeGlob})
	if spans := question.Match("cat"); len(spans) != 1 || spans[0].Text != "cat" {
		t.Fatalf("question spans %+v", spans)
	}
	if spans := question.Match("at"); len(spans) != 0 {
		t.Fatalf("question matched without a rune: %+v", spans)
	}
}

func TestGlobEscapeAndCase(t *testing.T) {
	escaped := compileMatcher(t, `cat\*`, MatcherOptions{Mode: ModeGlob})
	if spans := escaped.Match("cat*"); len(spans) != 1 {
		t.Fatalf("escaped star %+v", spans)
	}
	if spans := escaped.Match("catx"); len(spans) != 0 {
		t.Fatalf("escaped star matched a rune: %+v", spans)
	}
	ignoreCase := compileMatcher(t, "CAT*", MatcherOptions{Mode: ModeGlob, IgnoreCase: true})
	if spans := ignoreCase.Match("catfish"); len(spans) != 1 {
		t.Fatalf("ignore case glob %+v", spans)
	}
}

func TestRegexSpans(t *testing.T) {
	matcher := compileMatcher(t, "a+b", MatcherOptions{Mode: ModeRegex})
	got := matcher.Match("aab ab")
	want := []Span{span(1, 0, 3, "aab"), span(5, 4, 2, "ab")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("spans %+v", got)
	}
	anchored := compileMatcher(t, "^ab", MatcherOptions{Mode: ModeRegex})
	if spans := anchored.Match("abc ab"); len(spans) != 1 || spans[0].Offset != 0 {
		t.Fatalf("anchored spans %+v", spans)
	}
	ignoreCase := compileMatcher(t, "ab", MatcherOptions{Mode: ModeRegex, IgnoreCase: true})
	if spans := ignoreCase.Match("AB"); len(spans) != 1 {
		t.Fatalf("ignore case regex %+v", spans)
	}
}

func TestRegexEmptyMatch(t *testing.T) {
	matcher := compileMatcher(t, "x*", MatcherOptions{Mode: ModeRegex})
	got := matcher.Match("abc")
	if len(got) != 1 || got[0].Length != 0 || got[0].Offset != 0 {
		t.Fatalf("spans %+v", got)
	}
}

func TestFuzzyVariants(t *testing.T) {
	cases := []struct {
		query    string
		text     string
		distance int
		want     int
	}{
		{"hell", "hello", 1, 1},
		{"hallo", "hello", 1, 1},
		{"hxllx", "hello", 2, 1},
		{"abc", "xyz", 3, 1},
		{"abc", "xyz", 2, 0},
		{"grüse", "grüße", 1, 1},
		{"HELLO", "hello", 1, 1},
	}
	for _, test := range cases {
		matcher, err := Compile(test.query, MatcherOptions{
			Mode: ModeFuzzy, Fuzzy: test.distance, IgnoreCase: true,
		})
		if err != nil {
			t.Fatalf("%q: %v", test.query, err)
		}
		spans := matcher.Match(test.text)
		if len(spans) != test.want {
			t.Fatalf("%q on %q: %+v", test.query, test.text, spans)
		}
	}
	phrase, err := Compile("quick brown", MatcherOptions{Mode: ModeFuzzy, Fuzzy: 1})
	if err != nil {
		t.Fatal(err)
	}
	spans := phrase.Match("the quick, brown fox")
	if len(spans) != 2 || spans[0].Text != "quick" || spans[1].Text != "brown" {
		t.Fatalf("punctuation phrase %+v", spans)
	}
}

func TestMatcherRejects(t *testing.T) {
	if _, err := Compile("(", MatcherOptions{Mode: ModeRegex}); errorCode(err) != "search.invalid-regex" {
		t.Fatalf("invalid regex: %v", err)
	}
	if _, err := Compile(strings.Repeat("a", maxRegexBytes+1), MatcherOptions{Mode: ModeRegex}); errorCode(err) != "search.invalid-regex" {
		t.Fatalf("long regex: %v", err)
	}
	if _, err := Compile("cat", MatcherOptions{Mode: ModeGlob, Word: true}); errorCode(err) != "search.word-mode" {
		t.Fatalf("word with glob: %v", err)
	}
	if _, err := Compile("cat", MatcherOptions{Mode: Mode("bogus")}); errorCode(err) != "search.invalid-mode" {
		t.Fatalf("unknown mode: %v", err)
	}
}
