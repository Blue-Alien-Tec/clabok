package grammar

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPrepareTextMasksMarkup(t *testing.T) {
	input := "**Regelwerk:** Ein Satz.\n- Punkt\n> Zitat\n| a | b |\n" +
		"[Text](https://x.example) und <b>fett</b>.\n"
	prepared := prepareText(input)
	if len(prepared.text) != len(input) {
		t.Fatalf("the mask must preserve the byte count: %d != %d", len(prepared.text), len(input))
	}
	for _, marker := range []string{"**", "|", "](", "<b>", "(https"} {
		if strings.Contains(prepared.text, marker) {
			t.Fatalf("markup %q survived the prose view: %q", marker, prepared.text)
		}
	}
	for _, prose := range []string{"Regelwerk:", "Punkt", "Zitat", "Text", "fett"} {
		if !strings.Contains(prepared.text, prose) {
			t.Fatalf("prose %q was removed: %q", prose, prepared.text)
		}
	}
	at := strings.Index(prepared.text, "Regelwerk:")
	if at < 0 || prepared.original(at) != strings.Index(input, "Regelwerk:") {
		t.Fatalf("the offset map drifted")
	}
	if prepared.original(len(prepared.text)) != len(input) {
		t.Fatalf("the sentinel does not point at the body end")
	}
}

func TestPrepareTextKeepsIntraWordUnderscore(t *testing.T) {
	prepared := prepareText("snake_case und _betont_\n")
	if !strings.Contains(prepared.text, "snake_case") {
		t.Fatalf("an intra-word underscore is content: %q", prepared.text)
	}
	if strings.Contains(prepared.text, "_betont_") {
		t.Fatalf("an emphasis delimiter survived: %q", prepared.text)
	}
	if !strings.Contains(prepared.text, "betont") {
		t.Fatalf("the emphasized word was removed: %q", prepared.text)
	}
}

func TestCheckMasksMarkupBeforeTheRequest(t *testing.T) {
	var sent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/languages" {
			w.WriteHeader(http.StatusOK)
			return
		}
		_ = r.ParseForm()
		sent = r.Form.Get("text")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"matches":[]}`)
	}))
	defer server.Close()

	body := "**Regelwerk:** Ein Satz.\n- Punkt\n| a | b |\n" +
		"[Text](https://x.example) und <b>fett</b>.\n"
	result, err := NewClient(server.URL).Check(context.Background(), body, "de-DE")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 0 {
		t.Fatalf("findings %+v", result.Findings)
	}
	for _, marker := range []string{"*", "|", "](", "<b>", "(https"} {
		if strings.Contains(sent, marker) {
			t.Fatalf("the request still carries %q: %q", marker, sent)
		}
	}
	for _, prose := range []string{"Regelwerk:", "Punkt", "Text", "fett"} {
		if !strings.Contains(sent, prose) {
			t.Fatalf("the request lost the prose %q: %q", prose, sent)
		}
	}
}

// caseServer reports a DE_CASE finding on the word after the em dash of the fixture.
func caseServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/languages" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		sent := r.Form.Get("text")
		byteAt := strings.Index(sent, "Die")
		w.Header().Set("Content-Type", "application/json")
		if byteAt < 0 {
			fmt.Fprint(w, `{"matches":[]}`)
			return
		}
		at := len([]rune(sent[:byteAt]))
		fmt.Fprintf(w, `{"matches":[{"message":"case","offset":%d,"length":3,`+
			`"rule":{"id":"DE_CASE"},"replacements":[{"value":"die"}]}]}`, at)
	}))
}

func TestByteOffset(t *testing.T) {
	text := "aöb—c"
	cases := []struct {
		units int
		want  int
	}{{0, 0}, {1, 1}, {2, 3}, {3, 4}, {4, 7}, {5, 8}}
	for _, testCase := range cases {
		if got := byteOffset(text, 0, len(text), testCase.units); got != testCase.want {
			t.Fatalf("units %d: got %d, want %d", testCase.units, got, testCase.want)
		}
	}
	surrogate := "😀x"
	if got := byteOffset(surrogate, 0, len(surrogate), 1); got != 0 {
		t.Fatalf("a unit inside a surrogate pair keeps the offset before it: %d", got)
	}
	if got := byteOffset(surrogate, 0, len(surrogate), 2); got != 4 {
		t.Fatalf("two units end behind the pair: %d", got)
	}
}

// TestCheckMapsUtf16Offsets checks that the server's UTF-16 offsets land on the right body bytes when
// the prose carries multibyte characters before the finding.
func TestCheckMapsUtf16Offsets(t *testing.T) {
	body := "Grüße aus München, teh ist falsch.\n"
	units := len([]rune("Grüße aus München, "))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/languages" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"matches":[{"message":"typo","offset":%d,"length":3,`+
			`"rule":{"id":"TEST"},"replacements":[{"value":"der"}]}]}`, units)
	}))
	defer server.Close()

	result, err := NewClient(server.URL).Check(context.Background(), body, "de-DE")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 1 {
		t.Fatalf("findings %+v", result.Findings)
	}
	finding := result.Findings[0]
	want := strings.Index(body, "teh")
	if finding.Offset != want || finding.Offending != "teh" || finding.Line != 1 {
		t.Fatalf("finding %+v, want offset %d", finding, want)
	}
}

func TestCheckSuppressesArtefactAfterSpacedDash(t *testing.T) {
	server := caseServer(t)
	defer server.Close()

	result, err := NewClient(server.URL).Check(context.Background(),
		"Teil II — Die Galaxie\n", "de-DE")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 0 {
		t.Fatalf("the artefact reached the findings: %+v", result.Findings)
	}
	if len(result.Masked) != 1 || result.Masked[0].Rule != "DE_CASE" ||
		result.Masked[0].Count != 1 || result.Masked[0].Reason == "" {
		t.Fatalf("the suppression is not counted: %+v", result.Masked)
	}
}

func TestCheckKeepsCaseFindingWithoutDash(t *testing.T) {
	server := caseServer(t)
	defer server.Close()

	result, err := NewClient(server.URL).Check(context.Background(),
		"Teil II: Die Galaxie\n", "de-DE")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 1 || result.Findings[0].Rule != "DE_CASE" {
		t.Fatalf("a real case finding must survive: %+v", result.Findings)
	}
	if len(result.Masked) != 0 {
		t.Fatalf("nothing may be suppressed here: %+v", result.Masked)
	}
}
