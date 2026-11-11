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
	"time"

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
		if r.URL.Path == "/v2/languages" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `[{"code":"en-US","longCode":"en-US","name":"English (US)"}]`)
			return
		}
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
		if r.URL.Path == "/v2/languages" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `[{"code":"en-US","longCode":"en-US","name":"English (US)"}]`)
			return
		}
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

func TestGrammarCheckSkip(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	directory := t.TempDir()
	runOK(t, directory, "config", "set", "grammar.endpoint", server.URL)

	payload := dataOf(t, runOK(t, directory, "grammar", "check", "--raw", "teh cat"))
	if checked, _ := payload["checked"].(bool); checked {
		t.Fatalf("an unusable response must not look checked: %v", payload)
	}
	if reason, _ := payload["reason"].(string); reason != "grammar.response" {
		t.Fatalf("reason %v", payload["reason"])
	}
	warnings, _ := payload["warnings"].([]any)
	if len(warnings) != 1 {
		t.Fatalf("exactly one warning expected: %v", payload)
	}
	warning, _ := warnings[0].(map[string]any)
	if code, _ := warning["code"].(string); code != "grammar.response" {
		t.Fatalf("warning %v", warning)
	}

	code, _, stderr := run(t, directory, "grammar", "check", "--require-check", "--raw", "teh cat")
	if code != cli.ExitFailure || !strings.Contains(stderr, "grammar.check-skipped") {
		t.Fatalf("--require-check must fail: exit %d %s", code, stderr)
	}
}

func TestGrammarWriteSkipWarning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	directory := t.TempDir()
	runOK(t, directory, "config", "set", "grammar.endpoint", server.URL)
	runOK(t, directory, "book", "new", "--library", "--id", "library", "--title", "CLABOK")
	runOK(t, directory, "book", "new", "--title", "Guide")
	runOK(t, directory, "book", "open", "guide")
	chapter := dataOf(t, runOK(t, directory, "add", "--book", "guide", "--title", "Chapter"))["target"].(string)

	written := dataOf(t, runOK(t, directory, "body", "set", "--book", "guide", "0",
		"--parent", chapter, "--body", "teh cat"))
	report, _ := written["grammar"].(map[string]any)
	if report == nil {
		t.Fatalf("the skip report is missing: %v", written)
	}
	if checked, _ := report["checked"].(bool); checked {
		t.Fatalf("the write must carry an unchecked report: %v", report)
	}
	if reason, _ := report["reason"].(string); reason != "grammar.response" {
		t.Fatalf("reason %v", report["reason"])
	}
	warnings, _ := written["warnings"].([]any)
	if len(warnings) != 1 {
		t.Fatalf("exactly one warning expected: %v", written)
	}
	warning, _ := warnings[0].(map[string]any)
	if code, _ := warning["code"].(string); code != "grammar.response" {
		t.Fatalf("warning %v", warning)
	}
}

func TestGrammarServerInfoWatchdog(t *testing.T) {
	directory := t.TempDir()
	runOK(t, directory, "config", "set", "grammar.timeout", "2s")
	info := dataOf(t, runOK(t, directory, "grammar", "server-info"))
	if timeout, _ := info["timeout"].(string); timeout != "2s" {
		t.Fatalf("timeout %v", info["timeout"])
	}
	if cap, _ := info["cap"].(string); cap != "6s" {
		t.Fatalf("cap %v", info["cap"])
	}

	runOK(t, directory, "config", "set", "grammar.timeout", "1s")
	info = dataOf(t, runOK(t, directory, "grammar", "server-info"))
	if cap, _ := info["cap"].(string); cap != "3s" {
		t.Fatalf("cap after the interval change %v", info["cap"])
	}
}

func TestGrammarConfiguredTimeoutIsUsed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/languages" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `[{"code":"en-US","longCode":"en-US","name":"English (US)"}]`)
			return
		}
		time.Sleep(600 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"matches":[]}`)
	}))
	defer server.Close()

	directory := t.TempDir()
	runOK(t, directory, "config", "set", "grammar.endpoint", server.URL)
	runOK(t, directory, "config", "set", "grammar.timeout", "50ms")
	runOK(t, directory, "book", "new", "--library", "--id", "library", "--title", "CLABOK")
	runOK(t, directory, "book", "new", "--title", "Guide")
	runOK(t, directory, "book", "open", "guide")
	chapter := dataOf(t, runOK(t, directory, "add", "--book", "guide", "--title", "Chapter"))["target"].(string)

	written := dataOf(t, runOK(t, directory, "body", "set", "--book", "guide", "0",
		"--parent", chapter, "--body", "teh cat"))
	report, _ := written["grammar"].(map[string]any)
	if reason, _ := report["reason"].(string); reason != "grammar.timeout" {
		t.Fatalf("the configured interval must bound the check: %v", report)
	}
	warnings, _ := written["warnings"].([]any)
	if len(warnings) != 1 {
		t.Fatalf("warnings %v", written["warnings"])
	}
}

func TestGrammarLanguagesCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"code":"en-US","longCode":"en-US","name":"English (US)"},`+
			`{"code":"de-DE","longCode":"de-DE","name":"German (Germany)"}]`)
	}))
	defer server.Close()

	directory := t.TempDir()
	runOK(t, directory, "config", "set", "grammar.endpoint", server.URL)
	payload := dataOf(t, runOK(t, directory, "grammar", "languages"))
	if language, _ := payload["default"].(string); language != "en-US" {
		t.Fatalf("default %v", payload["default"])
	}
	if count, _ := payload["count"].(float64); count != 2 {
		t.Fatalf("count %v", payload["count"])
	}
	if cached, _ := payload["cached"].(bool); cached {
		t.Fatalf("first list must be live: %v", payload)
	}
	payload = dataOf(t, runOK(t, directory, "grammar", "languages"))
	if cached, _ := payload["cached"].(bool); !cached {
		t.Fatalf("second list must be cached: %v", payload)
	}

	runOK(t, directory, "book", "new", "--library", "--id", "library", "--title", "CLABOK")
	runOK(t, directory, "book", "new", "--title", "Guide")
	runOK(t, directory, "book", "open", "guide")
	runOK(t, directory, "book", "set", "guide", "language", "de-DE")
	payload = dataOf(t, runOK(t, directory, "grammar", "languages", "--book", "guide"))
	if language, _ := payload["bookLanguage"].(string); language != "de-DE" {
		t.Fatalf("book language %v", payload["bookLanguage"])
	}
	if supported, _ := payload["supported"].(bool); !supported {
		t.Fatalf("de-DE must be supported: %v", payload)
	}
}

func TestGrammarServerInfoIncludesLanguages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"code":"en-US","longCode":"en-US","name":"English (US)"},`+
			`{"code":"de-DE","longCode":"de-DE","name":"German (Germany)"}]`)
	}))
	defer server.Close()

	directory := t.TempDir()
	runOK(t, directory, "config", "set", "grammar.endpoint", server.URL)
	info := dataOf(t, runOK(t, directory, "grammar", "server-info"))
	if language, _ := info["language"].(string); language != "en-US" {
		t.Fatalf("default language %v", info["language"])
	}
	languages, _ := info["languages"].([]any)
	if len(languages) != 2 {
		t.Fatalf("languages %v", info["languages"])
	}
}

func TestGrammarCheckLanguageFlag(t *testing.T) {
	sent := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v2/languages" {
			fmt.Fprint(w, `[{"code":"en-US","longCode":"en-US","name":"English"},`+
				`{"code":"de-DE","longCode":"de-DE","name":"German"}]`)
			return
		}
		_ = r.ParseForm()
		sent = r.Form.Get("language")
		fmt.Fprint(w, `{"matches":[],"language":{"code":"de-DE",`+
			`"detectedLanguage":{"code":"de-DE"}}}`)
	}))
	defer server.Close()

	directory := t.TempDir()
	runOK(t, directory, "config", "set", "grammar.endpoint", server.URL)

	payload := dataOf(t, runOK(t, directory, "grammar", "check", "--raw", "teh cat",
		"--language", "de-DE"))
	if language, _ := payload["language"].(string); language != "de-DE" || sent != "de-DE" {
		t.Fatalf("language %v sent %q", payload["language"], sent)
	}
	payload = dataOf(t, runOK(t, directory, "grammar", "check", "--raw", "teh cat",
		"--language", "auto"))
	if language, _ := payload["language"].(string); language != "de-DE" || sent != "auto" {
		t.Fatalf("auto language %v sent %q", payload["language"], sent)
	}

	runOK(t, directory, "book", "new", "--library", "--id", "library", "--title", "CLABOK")
	runOK(t, directory, "book", "new", "--title", "Guide")
	runOK(t, directory, "book", "open", "guide")
	chapter := dataOf(t, runOK(t, directory, "add", "--book", "guide", "--title", "Chapter"))["target"].(string)
	written := dataOf(t, runOK(t, directory, "body", "set", "--book", "guide", "0",
		"--parent", chapter, "--body", "teh cat"))
	block, _ := written["block"].(string)
	code, _, stderr := run(t, directory, "grammar", "check", "--book", "guide", "--block", block,
		"--language", "de-DE")
	if code == cli.ExitOK || !strings.Contains(stderr, "usage.invalid-arguments") {
		t.Fatalf("--language with --book must be refused: exit %d %s", code, stderr)
	}
}

func TestBookLanguageSelectsFindings(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v2/languages" {
			fmt.Fprint(w, `[{"code":"en-US","longCode":"en-US","name":"English"},`+
				`{"code":"de-DE","longCode":"de-DE","name":"German"}]`)
			return
		}
		_ = r.ParseForm()
		if r.Form.Get("language") != "de-DE" {
			fmt.Fprint(w, `{"matches":[]}`)
			return
		}
		fmt.Fprint(w, `{"matches":[{"message":"German rule","offset":0,"length":3,`+
			`"rule":{"id":"DE_RULE"},"replacements":[{"value":"der"}]}]}`)
	}))
	defer server.Close()

	directory := t.TempDir()
	runOK(t, directory, "config", "set", "grammar.endpoint", server.URL)
	runOK(t, directory, "book", "new", "--library", "--id", "library", "--title", "CLABOK")
	runOK(t, directory, "book", "new", "--title", "Guide")
	runOK(t, directory, "book", "open", "guide")
	chapter := dataOf(t, runOK(t, directory, "add", "--book", "guide", "--title", "Chapter"))["target"].(string)

	runOK(t, directory, "book", "set", "guide", "language", "de-DE")
	code, _, stderr := run(t, directory, "body", "set", "--book", "guide", "0",
		"--parent", chapter, "--body", "teh cat")
	if code == cli.ExitOK || !strings.Contains(stderr, "body.grammar-blocked") {
		t.Fatalf("de-DE must block the English finding: exit %d %s", code, stderr)
	}

	runOK(t, directory, "book", "set", "guide", "language", "en-US")
	written := dataOf(t, runOK(t, directory, "body", "set", "--book", "guide", "0",
		"--parent", chapter, "--body", "teh cat"))
	report, _ := written["grammar"].(map[string]any)
	if language, _ := report["language"].(string); language != "en-US" {
		t.Fatalf("en-US report %v", report)
	}
	if checked, _ := report["checked"].(bool); !checked {
		t.Fatalf("en-US report %v", report)
	}
}

func TestTermsScopedByBook(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v2/languages" {
			fmt.Fprint(w, `[{"code":"en-US","longCode":"en-US","name":"English"},`+
				`{"code":"de-DE","longCode":"de-DE","name":"German"}]`)
			return
		}
		_ = r.ParseForm()
		text := r.Form.Get("text")
		matches := make([]map[string]any, 0)
		if at := strings.Index(text, "Simia"); at >= 0 {
			matches = append(matches, map[string]any{
				"message": "spelling", "offset": at, "length": 5,
				"rule":         map[string]any{"id": "SPELL"},
				"replacements": []map[string]any{{"value": "Simia"}},
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"matches": matches,
			"language": map[string]any{"code": "de-DE", "detectedLanguage": map[string]any{"code": "de-DE"}}})
	}))
	defer server.Close()

	directory := t.TempDir()
	runOK(t, directory, "config", "set", "grammar.endpoint", server.URL)
	runOK(t, directory, "book", "new", "--library", "--id", "library", "--title", "CLABOK")
	runOK(t, directory, "book", "new", "--title", "Guide")
	runOK(t, directory, "book", "open", "guide")
	runOK(t, directory, "book", "set", "guide", "language", "de-DE")

	request := dataOf(t, runOK(t, directory, "terms", "set", "--term", "Simia",
		"--reason", "ship name", "--book", "guide"))
	if scope, _ := request["scope"].(string); scope != "de-DE" {
		t.Fatalf("scope %v", request["scope"])
	}
	runOK(t, directory, "terms", "commit", "--id", request["id"].(string), "--confirm")

	languagePath := filepath.Join(directory, "_CLABOK", "terms", "de-de.txt")
	if data, err := os.ReadFile(languagePath); err != nil || string(data) != "Simia\n" {
		t.Fatalf("language layer %q %v", data, err)
	}

	checked := dataOf(t, runOK(t, directory, "grammar", "check", "--raw", "Simia flies.",
		"--language", "de-DE"))
	if count, _ := checked["count"].(float64); count != 0 {
		t.Fatalf("a suppressed finding must not count: %v", checked)
	}
	suppressed, _ := checked["suppressed"].([]any)
	if len(suppressed) != 1 {
		t.Fatalf("suppression attribution %v", checked["suppressed"])
	}
	entry, _ := suppressed[0].(map[string]any)
	if layer, _ := entry["suppressedBy"].(string); layer != "de-de" {
		t.Fatalf("suppression layer %v", entry)
	}

	listed := dataOf(t, runOK(t, directory, "terms", "list", "--language", "de-DE"))
	entries, _ := listed["entries"].([]any)
	if len(entries) != 1 {
		t.Fatalf("list entries %v", listed["entries"])
	}

	code, _, stderr := run(t, directory, "terms", "set", "--term", "X",
		"--reason", "no scope")
	if code != cli.ExitUsage || !strings.Contains(stderr, "usage.missing-argument") {
		t.Fatalf("a request without a scope must be refused: exit %d %s", code, stderr)
	}
}

func TestTermsCheckCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v2/languages" {
			fmt.Fprint(w, `[{"code":"en-US","longCode":"en-US","name":"English"},`+
				`{"code":"de-DE","longCode":"de-DE","name":"German"}]`)
			return
		}
		_ = r.ParseForm()
		if r.Form.Get("text") == "Rat" {
			fmt.Fprint(w, `{"matches":[]}`)
			return
		}
		fmt.Fprint(w, `{"matches":[{"message":"spelling","offset":0,"length":5,`+
			`"rule":{"id":"MORFOLOGIK_RULE_EN_US"},"replacements":[]}]}`)
	}))
	defer server.Close()

	directory := t.TempDir()
	runOK(t, directory, "config", "set", "grammar.endpoint", server.URL)
	runOK(t, directory, "book", "new", "--library", "--id", "library", "--title", "CLABOK")

	payload := dataOf(t, runOK(t, directory, "terms", "check", "--term", "Xyloq",
		"--scope", "en-US"))
	if tooBroad, _ := payload["tooBroad"].(bool); tooBroad {
		t.Fatalf("a misspelled name must be suppressible: %v", payload)
	}
	results, _ := payload["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("results %v", payload["results"])
	}

	payload = dataOf(t, runOK(t, directory, "terms", "check", "--term", "Rat",
		"--scope", "global"))
	if tooBroad, _ := payload["tooBroad"].(bool); !tooBroad {
		t.Fatalf("a real word must be reported as too broad: %v", payload)
	}
}

func TestTermsEndToEnd(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/languages" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `[{"code":"en-US","longCode":"en-US","name":"English (US)"}]`)
			return
		}
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
		"--reason", "the product name", "--scope", "global"))
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
		"--reason", "no longer needed", "--scope", "global"))
	runOK(t, directory, "terms", "commit", "--id", removed["id"].(string), "--confirm")
	checked = dataOf(t, runOK(t, directory, "grammar", "check", "--raw", "Clabok is the tool."))
	if count, _ := checked["count"].(float64); count != 1 {
		t.Fatalf("the removed term still suppresses: %v", checked)
	}

	extra := dataOf(t, runOK(t, directory, "terms", "set", "--term", "Temporary",
		"--reason", "test discard", "--scope", "global"))
	runOK(t, directory, "terms", "discard", "--id", extra["id"].(string))
	pending = dataOf(t, runOK(t, directory, "terms", "pending"))
	if entries, _ := pending["pending"].([]any); len(entries) != 0 {
		t.Fatalf("pending after discard %v", pending)
	}
}
