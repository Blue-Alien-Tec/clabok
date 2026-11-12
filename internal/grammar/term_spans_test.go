package grammar

import (
	"context"
	"strings"
	"testing"
)

func termSet(terms ...string) TermSet {
	return TermSet{Layers: []TermLayer{{Scope: ScopeGlobal, Terms: terms}}}
}

func TestTermSpansOccurrences(t *testing.T) {
	set := termSet("Unabhängiger Bund")
	text := "Die Zone des Unabhängiger Bund ist weit. Ein Unabhängiger Bund bleibt."
	spans := set.Spans(text)
	if len(spans) != 2 {
		t.Fatalf("spans %+v", spans)
	}
	first := strings.Index(text, "Unabhängiger Bund")
	if spans[0].Start != first || spans[0].End != first+len("Unabhängiger Bund") {
		t.Fatalf("first span %+v", spans[0])
	}
	second := strings.LastIndex(text, "Unabhängiger Bund")
	if spans[1].Start != second {
		t.Fatalf("second span %+v", spans[1])
	}
	if spans[0].Term != "Unabhängiger Bund" || spans[0].Layer != ScopeGlobal ||
		spans[0].Match != "word" {
		t.Fatalf("attribution %+v", spans[0])
	}
}

func TestTermSpansRespectBoundaries(t *testing.T) {
	if spans := termSet("Bad").Spans("Badwater und Badwater"); len(spans) != 0 {
		t.Fatalf("a term inside a longer word must not match: %+v", spans)
	}
	if spans := termSet("Bad").Spans("das Bad, das Bad"); len(spans) != 2 {
		t.Fatalf("bounded occurrences %+v", spans)
	}
	if spans := termSet("~ad").Spans("Badwater"); len(spans) != 1 ||
		spans[0].Term != "ad" || spans[0].Match != "substring" {
		t.Fatalf("substring form %+v", spans)
	}
}

func TestTermSpanSurvivesCaseMapping(t *testing.T) {
	text := "STRAẞE und danach Bad Lands"
	spans := termSet("straße", "Bad Lands").Spans(text)
	if len(spans) != 2 {
		t.Fatalf("spans %+v", spans)
	}
	if spans[0].Start != 0 || spans[0].End != len("STRAẞE") {
		t.Fatalf("the expanded case mapping shifted the span: %+v", spans[0])
	}
	if spans[1].Start != strings.Index(text, "Bad Lands") {
		t.Fatalf("the second span is off: %+v", spans[1])
	}
}

func TestTermSpansKeepTheFirstLayer(t *testing.T) {
	set := TermSet{Layers: []TermLayer{
		{Scope: ScopeBuiltin, Terms: []string{"Bad Lands"}},
		{Scope: ScopeGlobal, Terms: []string{"Bad Lands"}},
	}}
	spans := set.Spans("Jenseits der Bad Lands")
	if len(spans) != 1 {
		t.Fatalf("spans %+v", spans)
	}
	if spans[0].Layer != ScopeBuiltin || len(spans[0].AlsoIn) != 1 ||
		spans[0].AlsoIn[0] != ScopeGlobal {
		t.Fatalf("attribution %+v", spans[0])
	}
}

func TestCheckSuppressesCaseFindingInsideTerm(t *testing.T) {
	server := reportServer(t, func(prepared string) []map[string]any {
		at := strings.Index(prepared, "Unabhängigen")
		if at < 0 {
			return nil
		}
		return []map[string]any{matchAt("DE_CASE", "unabhängigen",
			unitsBefore(prepared, at),
			unitsBefore(prepared, at+len("Unabhängigen"))-unitsBefore(prepared, at))}
	})
	defer server.Close()

	client := NewClient(server.URL)
	client.Terms = termSet("Unabhängiger Bund", "Unabhängigen Bundes")
	result, err := client.Check(context.Background(),
		"Die Einflusszone des Unabhängigen Bundes ist weit.\n", DefaultLanguage)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 0 {
		t.Fatalf("the term must silence the rule: %+v", result.Findings)
	}
	if len(result.Suppressed) != 1 {
		t.Fatalf("suppressed %+v", result.Suppressed)
	}
	entry := result.Suppressed[0]
	if entry.Term != "Unabhängigen Bundes" || entry.Field != "span" ||
		entry.Layer != ScopeGlobal {
		t.Fatalf("entry %+v", entry)
	}
	if entry.Length != len("Unabhängigen") {
		t.Fatalf("the entry must carry the finding range: %+v", entry)
	}
}

func TestCheckSuppressesAgreementFindingOnArticle(t *testing.T) {
	server := reportServer(t, func(prepared string) []map[string]any {
		at := strings.Index(prepared, "der Bad")
		if at < 0 {
			return nil
		}
		return []map[string]any{matchAt("DE_AGREEMENT", "der Bäder",
			unitsBefore(prepared, at), len("der Bad"))}
	})
	defer server.Close()

	client := NewClient(server.URL)
	client.Terms = termSet("Bad Lands")
	result, err := client.Check(context.Background(),
		"Jenseits der Bad Lands beginnt nichts.\n", DefaultLanguage)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 0 || len(result.Suppressed) != 1 {
		t.Fatalf("findings %+v suppressed %+v", result.Findings, result.Suppressed)
	}
	if result.Suppressed[0].Term != "Bad Lands" {
		t.Fatalf("entry %+v", result.Suppressed[0])
	}
}

func TestCheckKeepsFindingOutsideTerm(t *testing.T) {
	server := reportServer(t, func(prepared string) []map[string]any {
		at := strings.Index(prepared, "Bund")
		if at < 0 {
			return nil
		}
		return []map[string]any{matchAt("DE_CASE", "bund", unitsBefore(prepared, at), 4)}
	})
	defer server.Close()

	client := NewClient(server.URL)
	client.Terms = termSet("Bad Lands")
	result, err := client.Check(context.Background(),
		"Der Bund ist alt. Bad Lands sind weit.\n", DefaultLanguage)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 1 || len(result.Suppressed) != 0 {
		t.Fatalf("findings %+v suppressed %+v", result.Findings, result.Suppressed)
	}
	if result.Findings[0].Offset != strings.Index("Der Bund ist alt. Bad Lands sind weit.\n", "Bund") {
		t.Fatalf("offset %d", result.Findings[0].Offset)
	}
}

func TestMaskWinsOverTermSpan(t *testing.T) {
	server := reportServer(t, func(prepared string) []map[string]any {
		at := strings.Index(prepared, "code")
		if at < 0 {
			return nil
		}
		return []map[string]any{matchAt("TEST_RULE", "Bad Lands", unitsBefore(prepared, at), 4)}
	})
	defer server.Close()

	client := NewClient(server.URL)
	client.Terms = termSet("Bad Lands")
	result, err := client.Check(context.Background(),
		"Ein Zitat `Bad Lands` steht hier.\n", DefaultLanguage)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 0 || len(result.Suppressed) != 0 {
		t.Fatalf("findings %+v suppressed %+v", result.Findings, result.Suppressed)
	}
	if len(result.Masked) != 1 || result.Masked[0].Rule != "TEST_RULE" {
		t.Fatalf("masked %+v", result.Masked)
	}
}
