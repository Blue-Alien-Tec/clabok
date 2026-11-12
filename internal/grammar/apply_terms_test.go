package grammar_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestApplyIgnoresSuppressedFindings pins the contract that a term only silences: a finding a term
// suppresses must never reach the repair path.
func TestApplyIgnoresSuppressedFindings(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/languages" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `[{"code":"de-DE","longCode":"de-DE","name":"German (Germany)"}]`)
			return
		}
		_ = r.ParseForm()
		text := r.Form.Get("text")
		matches := make([]map[string]any, 0, 1)
		if at := strings.Index(text, "Unabhängigen"); at >= 0 {
			matches = append(matches, map[string]any{
				"message": "case", "offset": at, "length": len([]rune("Unabhängigen")),
				"rule":         map[string]any{"id": "DE_CASE"},
				"replacements": []map[string]any{{"value": "unabhängigen"}},
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"matches": matches})
	}))
	defer server.Close()

	directory := t.TempDir()
	runOK(t, directory, "config", "set", "grammar.endpoint", server.URL)
	runOK(t, directory, "config", "set", "grammar.policy", "warn")
	runOK(t, directory, "book", "new", "--library", "--id", "library", "--title", "CLABOK")
	runOK(t, directory, "book", "new", "--title", "Guide")
	runOK(t, directory, "book", "open", "guide")
	termsPath := filepath.Join(directory, "_CLABOK", "terms.txt")
	if err := os.WriteFile(termsPath, []byte("Unabhängigen Bundes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	chapter := dataOf(t, runOK(t, directory, "add", "--book", "guide", "--title", "Chapter"))["target"].(string)
	body := "Die Einflusszone des Unabhängigen Bundes ist weit.\n"
	written := dataOf(t, runOK(t, directory, "body", "set", "--book", "guide", "0",
		"--parent", chapter, "--body", body))
	block, _ := written["block"].(string)

	applied := dataOf(t, runOK(t, directory, "grammar", "apply", "--book", "guide",
		"--block", block, "--all-safe"))
	if applied["applied"] != float64(0) || applied["changed"] != false {
		t.Fatalf("a suppressed finding reached the repair path: %v", applied)
	}
	if text := dataOf(t, runOK(t, directory, "body", "get", "--book", "guide", block))["text"]; text != body {
		t.Fatalf("the body changed: %q", text)
	}
}
