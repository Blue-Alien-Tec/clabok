package search

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
)

func TestMultiQueryORMatches(t *testing.T) {
	scene := newScene(t)
	result, err := scene.Service.Find(Options{
		Book: "fixture", Queries: []Query{{Text: "Hello"}, {Text: "drawn"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Matches) != 4 {
		t.Fatalf("matches %d", len(result.Matches))
	}
	payload := result.Payload(0, 0).(matchesPayload)
	if payload.Join != "or" || len(payload.Queries) != 2 || payload.Queries[1].Text != "drawn" {
		t.Fatalf("header %+v", payload.header)
	}
	for _, match := range result.Matches {
		if match.Block == "12" && match.Spans[0].Query != 1 {
			t.Fatalf("drawn span query %+v", match.Spans)
		}
	}
}

func TestMultiQueryANDMatches(t *testing.T) {
	scene := newScene(t)
	result, err := scene.Service.Find(Options{
		Book: "fixture", Join: "and",
		Queries: []Query{{Text: "Hello"}, {Text: "world"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Matches) != 1 {
		t.Fatalf("matches %+v", result.Matches)
	}
	match := result.Matches[0]
	if match.Block != "11" || match.Line != 1 || len(match.Spans) != 2 {
		t.Fatalf("match %+v", match)
	}
	if match.Spans[0].Query != 0 || match.Spans[1].Query != 1 {
		t.Fatalf("spans %+v", match.Spans)
	}
}

func TestMultiQueryANDAggregates(t *testing.T) {
	scene := newScene(t)
	queries := []Query{{Text: "Hello"}, {Text: "world"}}
	result, err := scene.Service.Find(Options{Book: "fixture", Join: "and", Queries: queries, Result: "blocks"})
	if err != nil {
		t.Fatal(err)
	}
	blocks := result.Payload(0, 0).(blocksPayload)
	if blocks.Count != 1 || len(blocks.Blocks) != 1 || blocks.Blocks[0].Block != "11" {
		t.Fatalf("blocks %+v", blocks.Blocks)
	}
	if len(blocks.Blocks[0].PerQuery) != 2 {
		t.Fatalf("perQuery %+v", blocks.Blocks[0].PerQuery)
	}
	nodes, err := scene.Service.Find(Options{Book: "fixture", Join: "and", Queries: queries, Result: "nodes"})
	if err != nil {
		t.Fatal(err)
	}
	nodePayload := nodes.Payload(0, 0).(nodesPayload)
	if nodePayload.Count != 1 || nodePayload.Nodes[0].Node != "10" {
		t.Fatalf("nodes %+v", nodePayload.Nodes)
	}
	books, err := scene.Service.Find(Options{Book: "fixture", Join: "and", Queries: queries, Result: "books"})
	if err != nil {
		t.Fatal(err)
	}
	bookPayload := books.Payload(0, 0).(booksPayload)
	if bookPayload.Count != 1 || bookPayload.Books[0].Count != 3 || bookPayload.Books[0].PerQuery[0].Count != 3 {
		t.Fatalf("books %+v", bookPayload.Books)
	}
}

func TestMultiQueryORBlocks(t *testing.T) {
	scene := newScene(t)
	result, err := scene.Service.Find(Options{
		Book: "fixture", Result: "blocks",
		Queries: []Query{{Text: "Hello"}, {Text: "drawn"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	blocks := result.Payload(0, 0).(blocksPayload)
	if blocks.Count != 3 {
		t.Fatalf("blocks %d: %+v", blocks.Count, blocks.Blocks)
	}
}

func TestMultiQueryRejects(t *testing.T) {
	scene := newScene(t)
	cases := []struct {
		options Options
		code    string
	}{
		{Options{Book: "fixture", Queries: []Query{{Text: "a"}, {Text: " a "}}}, "search.duplicate-query"},
		{Options{Book: "fixture", Queries: []Query{{Text: " "}}}, "search.empty-query"},
		{Options{Book: "fixture", Join: "xor", Queries: []Query{{Text: "a"}}}, "search.invalid-join"},
		{Options{Book: "fixture", Queries: makeQueries(9)}, "search.too-many-queries"},
	}
	for _, test := range cases {
		if _, err := scene.Service.Find(test.options); errorCode(err) != test.code {
			t.Fatalf("%+v: %v", test.options, err)
		}
	}
}

func makeQueries(count int) []Query {
	queries := make([]Query, count)
	for index := range queries {
		queries[index] = Query{Text: "q" + string(rune('a'+index))}
	}
	return queries
}

func TestFuzzyMatching(t *testing.T) {
	cases := []struct {
		query string
		text  string
		want  []string
	}{
		{"Helo", "Hello world", []string{"Hello"}},
		{"wrold", "hello world", []string{"world"}},
		{"quik brwn", "the quick brown fox", []string{"quick", "brown"}},
		{"xyz", "hello world", nil},
	}
	for _, test := range cases {
		matcher, err := Compile(test.query, MatcherOptions{Mode: ModeFuzzy, Fuzzy: 1})
		if err != nil {
			t.Fatalf("%q: %v", test.query, err)
		}
		spans := matcher.Match(test.text)
		texts := make([]string, len(spans))
		for index, span := range spans {
			texts[index] = span.Text
			if span.Distance == 0 {
				t.Fatalf("%q matched without a distance", test.query)
			}
		}
		if strings.Join(texts, " ") != strings.Join(test.want, " ") {
			t.Fatalf("%q: %v", test.query, texts)
		}
	}
}

func TestFuzzyOrderAndDistance(t *testing.T) {
	matcher, err := Compile("brown quick", MatcherOptions{Mode: ModeFuzzy, Fuzzy: 1})
	if err != nil {
		t.Fatal(err)
	}
	if spans := matcher.Match("the quick brown fox"); len(spans) != 0 {
		t.Fatalf("order-independent match %+v", spans)
	}
	if _, err := Compile("*", MatcherOptions{Mode: ModeFuzzy}); errorCode(err) != "search.invalid-fuzzy" {
		t.Fatalf("no word: %v", err)
	}
	words := strings.Repeat("word ", maxFuzzyWords+1)
	if _, err := Compile(words, MatcherOptions{Mode: ModeFuzzy}); errorCode(err) != "search.too-many-words" {
		t.Fatalf("too many words: %v", err)
	}
	if _, err := Compile("ab", MatcherOptions{Mode: ModeFuzzy, Word: true}); errorCode(err) != "search.fuzzy-word" {
		t.Fatalf("word with fuzzy: %v", err)
	}
	if _, err := Compile("ab", MatcherOptions{Mode: ModeGlob, Fuzzy: 1}); errorCode(err) != "search.fuzzy-mode" {
		t.Fatalf("glob with fuzzy: %v", err)
	}
}

func TestLiteralWithFuzzyOption(t *testing.T) {
	matcher, err := Compile("helo", MatcherOptions{Mode: ModeLiteral, Fuzzy: 1})
	if err != nil {
		t.Fatal(err)
	}
	if spans := matcher.Match("hello"); len(spans) != 1 || spans[0].Text != "hello" {
		t.Fatalf("spans %+v", spans)
	}
}

func TestFindCommandMultiQuery(t *testing.T) {
	scene := newScene(t)
	code, stdout, stderr := run(t, scene.Layout.Root,
		commandArgs(scene, "find", "--book", "fixture", "--query", "Hello", "--query", "drawn")...)
	if code != cli.ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var envelope struct {
		Data struct {
			Count   int         `json:"count"`
			Queries []QueryInfo `json:"queries"`
			Join    string      `json:"join"`
			Matches []Match     `json:"matches"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("payload %s: %v", stdout, err)
	}
	if envelope.Data.Count != 4 || len(envelope.Data.Queries) != 2 || envelope.Data.Join != "or" {
		t.Fatalf("payload %+v", envelope.Data)
	}
	code, stdout, stderr = run(t, scene.Layout.Root,
		commandArgs(scene, "find", "--book", "fixture", "--query", "Hello", "--query", "world", "--join", "and")...)
	if code != cli.ExitOK {
		t.Fatalf("and exit %d: %s", code, stderr)
	}
	var andEnvelope struct {
		Data struct {
			Count int `json:"count"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &andEnvelope); err != nil {
		t.Fatal(err)
	}
	if andEnvelope.Data.Count != 1 {
		t.Fatalf("and count %d", andEnvelope.Data.Count)
	}
}

func TestFindCommandFuzzyAndModes(t *testing.T) {
	scene := newScene(t)
	code, stdout, stderr := run(t, scene.Layout.Root,
		commandArgs(scene, "find", "--book", "fixture", "--query", "Helo", "--fuzzy", "1")...)
	if code != cli.ExitOK {
		t.Fatalf("fuzzy exit %d: %s", code, stderr)
	}
	var envelope struct {
		Data struct {
			Count   int         `json:"count"`
			Fuzzy   int         `json:"fuzzy"`
			Queries []QueryInfo `json:"queries"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.Count != 3 || envelope.Data.Fuzzy != 1 || envelope.Data.Queries[0].Mode != "literal" {
		t.Fatalf("fuzzy payload %+v", envelope.Data)
	}
	code, stdout, stderr = run(t, scene.Layout.Root,
		commandArgs(scene, "find", "--book", "fixture", "--query", "Hel+o", "--query", "drawn",
			"--query-mode", "regex", "--query-mode", "literal")...)
	if code != cli.ExitOK {
		t.Fatalf("mixed exit %d: %s", code, stderr)
	}
	var mixed struct {
		Data struct {
			Count   int         `json:"count"`
			Queries []QueryInfo `json:"queries"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &mixed); err != nil {
		t.Fatal(err)
	}
	if mixed.Data.Count != 4 || mixed.Data.Queries[0].Mode != "regex" || mixed.Data.Queries[1].Mode != "literal" {
		t.Fatalf("mixed payload %+v", mixed.Data)
	}
	code, _, stderr = run(t, scene.Layout.Root,
		commandArgs(scene, "find", "--book", "fixture", "--query", "a", "--query", "b", "--query-mode", "literal")...)
	if code != cli.ExitUsage {
		t.Fatalf("query-mode count exit %d: %s", code, stderr)
	}
}
