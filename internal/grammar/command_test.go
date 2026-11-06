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

	"github.com/Blue-Alien-Tec/clabok/internal/body"
	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/config"
	"github.com/Blue-Alien-Tec/clabok/internal/coordinator"
	"github.com/Blue-Alien-Tec/clabok/internal/grammar"
	"github.com/Blue-Alien-Tec/clabok/internal/library"
	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/structure"
)

func newRegistry() *cli.Registry {
	registry := cli.NewRegistry()
	config.Register(registry)
	coordinator.Register(registry)
	lifecycle.Register(registry)
	structure.Register(registry)
	library.Register(registry)
	body.Register(registry)
	grammar.Register(registry)
	return registry
}

func run(t *testing.T, directory string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr strings.Builder
	code := cli.Run(newRegistry(), append([]string{"--project", directory}, args...), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func runOK(t *testing.T, directory string, args ...string) string {
	t.Helper()
	code, stdout, stderr := run(t, directory, args...)
	if code != cli.ExitOK {
		t.Fatalf("%v: exit %d\n%s%s", args, code, stdout, stderr)
	}
	return stdout
}

func dataOf(t *testing.T, stdout string) map[string]any {
	t.Helper()
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("parse %q: %v", stdout, err)
	}
	return envelope.Data
}

func TestGrammarGateEndToEnd(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"matches":[{"message":"Possible typo","offset":4,"length":3,`+
			`"rule":{"id":"MORFOLOGIK_RULE_EN_US"},"replacements":[{"value":"the"}]}]}`)
	}))
	defer server.Close()

	directory := t.TempDir()
	runOK(t, directory, "config", "set", "grammar.endpoint", server.URL)
	runOK(t, directory, "book", "new", "--library", "--id", "library", "--title", "CLABOK")
	runOK(t, directory, "book", "new", "--title", "Guide")
	runOK(t, directory, "book", "open", "guide")
	chapter := dataOf(t, runOK(t, directory, "add", "--book", "guide", "--title", "Chapter"))["target"].(string)

	code, _, stderr := run(t, directory, "body", "set", "--book", "guide", "0",
		"--parent", chapter, "--body", "teh cat")
	if code == cli.ExitOK || !strings.Contains(stderr, `"body.grammar-blocked"`) {
		t.Fatalf("a block finding must refuse the write: %s", stderr)
	}

	runOK(t, directory, "config", "set", "grammar.policy", "warn")
	written := dataOf(t, runOK(t, directory, "body", "set", "--book", "guide", "0",
		"--parent", chapter, "--body", "teh cat"))
	block, _ := written["block"].(string)
	if block == "" {
		t.Fatalf("write payload %v", written)
	}
	report, _ := written["grammar"].(map[string]any)
	if report == nil {
		t.Fatalf("the warn report is missing: %v", written)
	}

	checked := dataOf(t, runOK(t, directory, "grammar", "check", "--book", "guide", "--block", block))
	if count, _ := checked["count"].(float64); count != 1 {
		t.Fatalf("check payload %v", checked)
	}

	raw := dataOf(t, runOK(t, directory, "grammar", "check", "--raw", "abc\nteh cat"))
	items, _ := raw["findings"].([]any)
	if len(items) != 1 {
		t.Fatalf("raw check payload %v", raw)
	}
	finding, _ := items[0].(map[string]any)
	if line, _ := finding["line"].(float64); line != 2 {
		t.Fatalf("line %v", finding["line"])
	}
}

func TestGrammarApplyEndToEnd(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		text := r.Form.Get("text")
		matches := make([]map[string]any, 0)
		for _, pair := range []struct{ word, replacement string }{{"teh", "the"}, {"cat", "dog"}} {
			if at := strings.Index(text, pair.word); at >= 0 {
				matches = append(matches, map[string]any{
					"message":      "typo",
					"offset":       at,
					"length":       len(pair.word),
					"rule":         map[string]any{"id": "TEST_RULE"},
					"replacements": []map[string]any{{"value": pair.replacement}},
				})
			}
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
	chapter := dataOf(t, runOK(t, directory, "add", "--book", "guide", "--title", "Chapter"))["target"].(string)
	written := dataOf(t, runOK(t, directory, "body", "set", "--book", "guide", "0",
		"--parent", chapter, "--body", "teh cat"))
	block, _ := written["block"].(string)

	applied := dataOf(t, runOK(t, directory, "grammar", "apply", "--book", "guide",
		"--block", block, "--all-safe", "--expect", "2"))
	if applied["applied"] != float64(2) {
		t.Fatalf("apply payload %v", applied)
	}
	if text := dataOf(t, runOK(t, directory, "body", "get", "--book", "guide", block))["text"]; text != "the dog" {
		t.Fatalf("applied body %q", text)
	}

	runOK(t, directory, "body", "set", "--book", "guide", block, "--body", "teh cat")
	applied = dataOf(t, runOK(t, directory, "grammar", "apply", "--book", "guide",
		"--block", block, "--finding", "1", "--expect", "1"))
	if applied["applied"] != float64(1) {
		t.Fatalf("partial apply payload %v", applied)
	}
	if text := dataOf(t, runOK(t, directory, "body", "get", "--book", "guide", block))["text"]; text != "the cat" {
		t.Fatalf("partial body %q", text)
	}
	if remaining, _ := applied["remaining"].([]any); len(remaining) != 1 {
		t.Fatalf("remaining %v", applied["remaining"])
	}

	runOK(t, directory, "body", "set", "--book", "guide", block, "--body", "teh cat")
	runOK(t, directory, "grammar", "apply", "--book", "guide", "--block", block, "--all-safe", "--dry-run")
	if text := dataOf(t, runOK(t, directory, "body", "get", "--book", "guide", block))["text"]; text != "teh cat" {
		t.Fatalf("a dry run wrote the body: %q", text)
	}

	code, _, stderr := run(t, directory, "grammar", "apply", "--book", "guide",
		"--block", block, "--all-safe", "--expect", "1")
	if code == cli.ExitOK || !strings.Contains(stderr, "grammar.expectation-failed") {
		t.Fatalf("expectation guard: %s", stderr)
	}
}

func TestTermsEndToEnd(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		text := r.Form.Get("text")
		matches := make([]map[string]any, 0)
		if at := strings.Index(text, "Clabok"); at >= 0 {
			matches = append(matches, map[string]any{
				"message": "possible spelling", "offset": at, "length": 6,
				"rule":         map[string]any{"id": "TEST_TERM"},
				"replacements": []map[string]any{{"value": "CLABOK"}},
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"matches": matches})
	}))
	defer server.Close()

	directory := t.TempDir()
	runOK(t, directory, "config", "set", "grammar.endpoint", server.URL)
	request := dataOf(t, runOK(t, directory, "terms", "set", "--term", "Clabok",
		"--reason", "the product name"))
	id, _ := request["id"].(string)
	if id == "" {
		t.Fatalf("request payload %v", request)
	}
	termsPath := filepath.Join(directory, "_CLABOK", "terms.txt")
	if _, err := os.Stat(termsPath); !os.IsNotExist(err) {
		t.Fatalf("a request wrote the term list: %v", err)
	}

	pending := dataOf(t, runOK(t, directory, "terms", "pending"))
	if entries, _ := pending["pending"].([]any); len(entries) != 1 {
		t.Fatalf("pending payload %v", pending)
	}

	code, _, stderr := run(t, directory, "terms", "commit", "--id", id)
	if code == cli.ExitOK || !strings.Contains(stderr, "terms.confirmation-required") {
		t.Fatalf("commit without confirm: %s", stderr)
	}
	runOK(t, directory, "terms", "commit", "--id", id, "--confirm")
	if data, err := os.ReadFile(termsPath); err != nil || string(data) != "Clabok\n" {
		t.Fatalf("terms %q %v", data, err)
	}

	checked := dataOf(t, runOK(t, directory, "grammar", "check", "--raw", "Clabok is the tool."))
	if count, _ := checked["count"].(float64); count != 0 {
		t.Fatalf("the term did not suppress the finding: %v", checked)
	}

	removed := dataOf(t, runOK(t, directory, "terms", "delete", "--term", "Clabok",
		"--reason", "no longer needed"))
	runOK(t, directory, "terms", "commit", "--id", removed["id"].(string), "--confirm")
	checked = dataOf(t, runOK(t, directory, "grammar", "check", "--raw", "Clabok is the tool."))
	if count, _ := checked["count"].(float64); count != 1 {
		t.Fatalf("the removed term still suppresses: %v", checked)
	}

	extra := dataOf(t, runOK(t, directory, "terms", "set", "--term", "Temporary",
		"--reason", "test discard"))
	runOK(t, directory, "terms", "discard", "--id", extra["id"].(string))
	pending = dataOf(t, runOK(t, directory, "terms", "pending"))
	if entries, _ := pending["pending"].([]any); len(entries) != 0 {
		t.Fatalf("pending after discard %v", pending)
	}
}
