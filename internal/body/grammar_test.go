package body

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/grammar"
)

func grammarServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"matches":[{"message":"Possible typo","offset":0,"length":3,`+
			`"rule":{"id":"MORFOLOGIK_RULE_EN_US"},"replacements":[{"value":"the"}]}]}`)
	}))
}

func TestGrammarGateReplace(t *testing.T) {
	server := grammarServer(t)
	defer server.Close()
	scene := newScene(t)
	scene.service.Grammar = grammar.NewGate(server.URL, grammar.PolicyWarn, nil)

	result, err := scene.service.Replace("fixture", 11,
		[]Pair{{Find: "Body", Replace: "teh"}}, ReplaceOptions{Expect: -1}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Grammar.Findings) != 1 {
		t.Fatalf("replace report %+v", result.Grammar)
	}

	scene.service.Grammar.Policy = grammar.PolicyBlock
	if _, err := scene.service.Replace("fixture", 11,
		[]Pair{{Find: "11", Replace: "22"}}, ReplaceOptions{Expect: -1}, false); err == nil {
		t.Fatal("expected body.grammar-blocked")
	} else if coded, ok := err.(*Error); !ok || coded.Code != "body.grammar-blocked" {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestGrammarGateMatrix(t *testing.T) {
	server := grammarServer(t)
	defer server.Close()
	scene := newScene(t)
	path := filepath.Join(scene.paths.BlocksDir, "11.md")
	body := func() string {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}

	scene.service.Grammar = grammar.NewGate(server.URL, grammar.PolicyBlock, nil)
	if _, err := scene.service.Set("fixture", 11, 0, "teh cat", false); err == nil {
		t.Fatal("expected body.grammar-blocked")
	} else if coded, ok := err.(*Error); !ok || coded.Code != "body.grammar-blocked" ||
		coded.Details == nil {
		t.Fatalf("unexpected error %v", err)
	}
	if body() != "Body 11\n" {
		t.Fatalf("a blocked write changed the body: %q", body())
	}

	if _, err := scene.service.Set("fixture", 11, 0, "teh cat", true); err == nil {
		t.Fatal("a dry run must apply the same policy")
	}

	scene.service.Grammar.Policy = grammar.PolicyWarn
	result, err := scene.service.Set("fixture", 11, 0, "teh cat", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Grammar.Findings) != 1 {
		t.Fatalf("warn report %+v", result.Grammar)
	}
	if body() != "teh cat" {
		t.Fatalf("a warn write did not happen: %q", body())
	}

	server.Close()
	scene.service.Grammar = grammar.NewGate(server.URL, grammar.PolicyBlock, nil)
	scene.service.Grammar.Start = func(string) error {
		return errors.New("no server")
	}
	result, err = scene.service.Set("fixture", 11, 0, "teh cat", false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Grammar.Degraded == "" {
		t.Fatalf("an unavailable server must degrade: %+v", result.Grammar)
	}
	if body() != "teh cat" {
		t.Fatalf("a degraded check must not block: %q", body())
	}
}
