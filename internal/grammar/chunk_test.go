package grammar

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestChunkPreparedPacksParagraphs(t *testing.T) {
	text := "aaa\n\nbbb\n\nccc"
	chunks := chunkPrepared(text, 8)
	if len(chunks) != 2 {
		t.Fatalf("chunks %+v", chunks)
	}
	if got := text[chunks[0].start:chunks[0].end]; got != "aaa\n\nbbb" {
		t.Fatalf("first chunk %q", got)
	}
	if got := text[chunks[1].start:chunks[1].end]; got != "ccc" {
		t.Fatalf("second chunk %q", got)
	}
}

func TestChunkPreparedKeepsOversizedParagraph(t *testing.T) {
	text := strings.Repeat("x", 20) + "\n\nshort"
	chunks := chunkPrepared(text, 10)
	if len(chunks) != 2 {
		t.Fatalf("chunks %+v", chunks)
	}
	if chunks[0].end-chunks[0].start != 20 {
		t.Fatalf("an oversized paragraph must stay one chunk: %+v", chunks[0])
	}
}

func TestChunkPreparedEmpty(t *testing.T) {
	for _, text := range []string{"", "   ", "\n\n\n"} {
		if chunks := chunkPrepared(text, chunkBytes); len(chunks) != 0 {
			t.Fatalf("%q: chunks %+v", text, chunks)
		}
	}
}

// typoServer reports every occurrence of "teh" in the text it receives, relative to that text.
func typoServer(t *testing.T, calls *int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/languages" {
			w.WriteHeader(http.StatusOK)
			return
		}
		atomic.AddInt32(calls, 1)
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		text := r.Form.Get("text")
		parts := make([]string, 0)
		for at := strings.Index(text, "teh"); at >= 0; {
			parts = append(parts, fmt.Sprintf(
				`{"message":"Possible typo","offset":%d,"length":3,`+
					`"rule":{"id":"MORFOLOGIK_RULE_EN_US"},"replacements":[{"value":"the"}]}`, at))
			next := strings.Index(text[at+3:], "teh")
			if next < 0 {
				break
			}
			at += 3 + next
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"matches":[%s]}`, strings.Join(parts, ","))
	}))
}

func TestCheckChunkedOffsetsMatchTheBody(t *testing.T) {
	var calls int32
	server := typoServer(t, &calls)
	defer server.Close()

	var builder strings.Builder
	expected := make([]int, 0)
	for index := 0; index < 80; index++ {
		line := fmt.Sprintf("Paragraph %02d has some prose about a cat and a dog.\n\n", index)
		builder.WriteString(line)
		if index%4 == 0 {
			builder.WriteString("This paragraph has teh typo.\n\n")
			expected = append(expected, builder.Len()-len("This paragraph has teh typo.\n\n")+
				len("This paragraph has "))
		}
	}
	body := builder.String()
	if len(body) <= chunkBytes {
		t.Fatalf("the fixture must exceed one chunk: %d bytes", len(body))
	}

	findings, err := NewClient(server.URL, nil).Check(context.Background(), body)
	if err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&calls); got < 2 {
		t.Fatalf("the fixture must chunk, got %d request(s)", got)
	}
	if len(findings) != len(expected) {
		t.Fatalf("findings %d, expected %d: %+v", len(findings), len(expected), findings)
	}
	for index, finding := range findings {
		if finding.Offset != expected[index] {
			t.Fatalf("finding %d: offset %d, expected %d", index, finding.Offset, expected[index])
		}
		if finding.Index != index+1 {
			t.Fatalf("finding %d: index %d", index, finding.Index)
		}
		if finding.Offending != "teh" {
			t.Fatalf("finding %d: offending %q", index, finding.Offending)
		}
	}
}

func TestCheckOversizedParagraphIsOneRequest(t *testing.T) {
	var calls int32
	server := typoServer(t, &calls)
	defer server.Close()
	body := "teh " + strings.Repeat("word ", 1200)
	if len(body) <= chunkBytes {
		t.Fatalf("fixture too small: %d", len(body))
	}
	findings, err := NewClient(server.URL, nil).Check(context.Background(), body)
	if err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("an oversized paragraph is one request, got %d", got)
	}
	if len(findings) != 1 || findings[0].Offset != 0 {
		t.Fatalf("findings %+v", findings)
	}
}

func TestCheckChunkFailureDiscardsAllFindings(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/languages" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if atomic.AddInt32(&calls, 1) > 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, slowMatchPayload)
	}))
	defer server.Close()

	body := "teh first\n\nsecond paragraph\n\nthird paragraph\n\n" + strings.Repeat("more prose\n\n", 500)
	findings, err := NewClient(server.URL, nil).Check(context.Background(), body)
	if err == nil {
		t.Fatalf("a failed chunk must fail the whole check, findings %+v", findings)
	}
	if !errors.Is(err, ErrUnavailable) || ReasonFor(err) != ReasonResponse {
		t.Fatalf("expected a response failure, got %v", err)
	}
	if findings != nil {
		t.Fatalf("no partial findings may be returned: %+v", findings)
	}
}

func TestCheckEmptyBodySendsNoRequest(t *testing.T) {
	var calls int32
	server := typoServer(t, &calls)
	defer server.Close()
	client := NewClient(server.URL, nil)
	for _, body := range []string{"", "   ", "\n\n\n"} {
		findings, err := client.Check(context.Background(), body)
		if err != nil || len(findings) != 0 {
			t.Fatalf("%q: findings %+v, err %v", body, findings, err)
		}
	}
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Fatalf("an empty body must not reach the server, got %d requests", got)
	}
}

func TestCheckChunkedCodeSpanIsFiltered(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/languages" {
			w.WriteHeader(http.StatusOK)
			return
		}
		_ = r.ParseForm()
		at := strings.Index(r.Form.Get("text"), "code")
		w.Header().Set("Content-Type", "application/json")
		if at < 0 {
			fmt.Fprint(w, `{"matches":[]}`)
			return
		}
		fmt.Fprintf(w, `{"matches":[{"message":"prose","offset":%d,"length":4,`+
			`"rule":{"id":"TEST"},"replacements":[{"value":"x"}]}]}`, at)
	}))
	defer server.Close()

	body := strings.Repeat("plain paragraph text\n\n", 250) + "Inline `teh` code.\n"
	findings, err := NewClient(server.URL, nil).Check(context.Background(), body)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("a code span must not produce a finding: %+v", findings)
	}
}

func TestCheckFencedBlockAcrossParagraphs(t *testing.T) {
	var calls int32
	server := typoServer(t, &calls)
	defer server.Close()

	body := strings.Repeat("plain paragraph text\n\n", 250) +
		"```text\nteh inside a fence\n```\n" + "tail paragraph\n"
	findings, err := NewClient(server.URL, nil).Check(context.Background(), body)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("a fenced block must not produce a finding: %+v", findings)
	}
	if atomic.LoadInt32(&calls) == 0 {
		t.Fatal("the body must still be checked")
	}
}

// TestGrammarCliffFixtures covers the former size cliff: a body below the old limit and one above
// it are both checked completely, with body offsets and no silent skip.
func TestGrammarCliffFixtures(t *testing.T) {
	var calls int32
	server := typoServer(t, &calls)
	defer server.Close()
	client := NewClient(server.URL, nil)

	for _, size := range []struct {
		name       string
		paragraphs int
		words      int
	}{
		{name: "below", paragraphs: 450, words: 3600},
		{name: "above", paragraphs: 530, words: 4240},
	} {
		var builder strings.Builder
		for index := 0; index < size.paragraphs; index++ {
			fmt.Fprintf(&builder, "Paragraph %d has teh typo inside its prose line.\n\n", index)
		}
		body := builder.String()
		findings, err := client.Check(context.Background(), body)
		if err != nil {
			t.Fatalf("%s: %v", size.name, err)
		}
		if len(findings) != size.paragraphs {
			t.Fatalf("%s: %d findings for %d paragraphs", size.name, len(findings), size.paragraphs)
		}
		at := -1
		for index, finding := range findings {
			next := strings.Index(body[at+1:], "teh")
			if next < 0 {
				t.Fatalf("%s: fixture exhausted at finding %d", size.name, index)
			}
			at += 1 + next
			if finding.Offset != at {
				t.Fatalf("%s: finding %d offset %d, expected %d", size.name, index, finding.Offset, at)
			}
		}
	}
	if got := atomic.LoadInt32(&calls); got < 4 {
		t.Fatalf("both cliff fixtures must chunk, got %d request(s)", got)
	}
}

func TestMergeFindingsOrderAndDedupe(t *testing.T) {
	input := []Finding{
		{Offset: 9, Length: 3, Rule: "B", Message: "later"},
		{Offset: 2, Length: 3, Rule: "A", Message: "dupe"},
		{Offset: 9, Length: 3, Rule: "A", Message: "later"},
		{Offset: 2, Length: 3, Rule: "A", Message: "dupe"},
		{Offset: 2, Length: 4, Rule: "A", Message: "longer"},
	}
	merged := mergeFindings(input)
	if len(merged) != 4 {
		t.Fatalf("merged %+v", merged)
	}
	order := []struct {
		offset int
		length int
	}{
		{2, 3}, {2, 4}, {9, 3}, {9, 3},
	}
	for index, want := range order {
		if merged[index].Offset != want.offset || merged[index].Length != want.length {
			t.Fatalf("index %d: %+v", index, merged[index])
		}
	}
	if merged[2].Rule != "A" || merged[3].Rule != "B" {
		t.Fatalf("rule order %+v", merged)
	}
}
