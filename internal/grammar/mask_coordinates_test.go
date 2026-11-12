package grammar

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// reportServer answers a check with the findings a test derives from the text it received. The
// callback sees the prepared text, which is what a real server sees, so a test can report an offset
// exactly where the masker put the substituted bytes.
func reportServer(t *testing.T, build func(prepared string) []map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/languages" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `[{"code":"en-US","longCode":"en-US","name":"English (US)"}]`)
			return
		}
		_ = r.ParseForm()
		matches := build(r.Form.Get("text"))
		if matches == nil {
			matches = make([]map[string]any, 0)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"matches": matches})
	}))
}

func matchAt(rule, replacement string, offset, length int) map[string]any {
	return map[string]any{
		"message": rule, "offset": offset, "length": length,
		"rule":         map[string]any{"id": rule},
		"replacements": []map[string]any{{"value": replacement}},
	}
}

// unitsBefore converts a byte offset into the UTF-16 code units the server speaks, so a test can
// report an offset the way LanguageTool does.
func unitsBefore(text string, at int) int {
	return len([]rune(text[:at]))
}

func TestCheckDropsMarkerFindingBehindCodeSpan(t *testing.T) {
	server := reportServer(t, func(prepared string) []map[string]any {
		// The marker is masked into two spaces; the reported offset is prepared space, like a server.
		at := strings.Index(prepared, "  Status: open")
		if at < 0 {
			return nil
		}
		return []map[string]any{matchAt("WHITESPACE_RULE", " ", unitsBefore(prepared, at), 2)}
	})
	defer server.Close()

	body := "The command `pecia add --book x --title y` writes a unit.\n- Status: open\n"
	result, err := NewClient(server.URL).Check(context.Background(), body, DefaultLanguage)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 0 {
		t.Fatalf("a masked marker must not surface: %+v", result.Findings)
	}
	if len(result.Masked) != 1 || result.Masked[0].Rule != "WHITESPACE_RULE" ||
		result.Masked[0].Count != 1 || result.Masked[0].Reason == "" {
		t.Fatalf("a dropped finding must be counted with a reason: %+v", result.Masked)
	}
}

func TestCheckDropsSentenceStartArtifact(t *testing.T) {
	server := reportServer(t, func(prepared string) []map[string]any {
		// The masker substitutes the lowercase word "code" for code spans, so a code span that opens
		// a sentence trips the uppercase rule inside masked bytes.
		at := strings.Index(prepared, ". code")
		if at < 0 {
			return nil
		}
		return []map[string]any{matchAt("UPPERCASE_SENTENCE_START", "Code",
			unitsBefore(prepared, at+2), 1)}
	})
	defer server.Close()

	body := "A sentence ends here. `Client.finding` maps the range.\n"
	result, err := NewClient(server.URL).Check(context.Background(), body, DefaultLanguage)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 0 {
		t.Fatalf("a code span at a sentence start must not surface: %+v", result.Findings)
	}
	if len(result.Masked) != 1 || result.Masked[0].Rule != "UPPERCASE_SENTENCE_START" {
		t.Fatalf("masked %+v", result.Masked)
	}
}

func TestCheckKeepsProseFindingBesideMask(t *testing.T) {
	server := reportServer(t, func(prepared string) []map[string]any {
		at := strings.Index(prepared, "refuses the write")
		if at < 0 {
			return nil
		}
		return []map[string]any{matchAt("A_INFINITIVE", "writing",
			unitsBefore(prepared, at+8), 9)}
	})
	defer server.Close()

	body := "The gate `pecia grammar` refuses the write.\n"
	result, err := NewClient(server.URL).Check(context.Background(), body, DefaultLanguage)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 1 || result.Findings[0].Rule != "A_INFINITIVE" {
		t.Fatalf("a prose finding next to a mask must stay: %+v", result.Findings)
	}
	if result.Findings[0].Offset != strings.Index(body, "the write") {
		t.Fatalf("offset %d", result.Findings[0].Offset)
	}
	if len(result.Masked) != 0 {
		t.Fatalf("nothing may be counted as masked here: %+v", result.Masked)
	}
}

func TestCheckKeepsMultibyteMaskDrop(t *testing.T) {
	server := reportServer(t, func(prepared string) []map[string]any {
		at := strings.Index(prepared, "  Status: offen")
		if at < 0 {
			return nil
		}
		return []map[string]any{matchAt("WHITESPACE_RULE", " ", unitsBefore(prepared, at), 2)}
	})
	defer server.Close()

	body := "Grüße aus München, `pecia add --title Grüße` schreibt.\n- Status: offen\n"
	result, err := NewClient(server.URL).Check(context.Background(), body, DefaultLanguage)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 0 {
		t.Fatalf("the multibyte case regressed: %+v", result.Findings)
	}
}
