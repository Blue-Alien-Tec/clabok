package editorial

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
)

func TestEditorialProseGate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/languages" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `[{"code":"en-US","longCode":"en-US","name":"English (US)"}]`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"matches":[{"message":"Possible typo","offset":0,"length":3,`+
			`"rule":{"id":"MORFOLOGIK_RULE_EN_US"},"replacements":[{"value":"the"}]}]}`)
	}))
	defer server.Close()

	directory := t.TempDir()
	runOK(t, directory, "config", "set", "grammar.endpoint", server.URL)
	runOK(t, directory, "book", "new", "--library", "--id", "library", "--title", "CLABOK")
	runOK(t, directory, "book", "new", "--title", "Guide")
	runOK(t, directory, "book", "open", "guide")
	chapter := dataOf(t, runOK(t, directory, "add", "--book", "guide", "--title", "Chapter"))["target"].(string)

	code, _, stderr := run(t, directory, "prose", "set", "--book", "guide", "--node", chapter, "--prose", "teh cat")
	if code == cli.ExitOK || !strings.Contains(stderr, `"body.grammar-blocked"`) {
		t.Fatalf("prose set was not gated: %s", stderr)
	}

	runOK(t, directory, "config", "set", "grammar.policy", "warn")
	payload := dataOf(t, runOK(t, directory, "prose", "set", "--book", "guide", "--node", chapter, "--prose", "teh cat"))
	if report, _ := payload["grammar"].(map[string]any); report == nil {
		t.Fatalf("the warn report is missing: %v", payload)
	}

	runOK(t, directory, "config", "set", "grammar.policy", "block")
	code, _, stderr = run(t, directory, "region", "set", "--book", "guide", "--kind", "footer", "--body", "teh cat")
	if code == cli.ExitOK || !strings.Contains(stderr, `"body.grammar-blocked"`) {
		t.Fatalf("region set was not gated: %s", stderr)
	}

	runOK(t, directory, "config", "set", "grammar.policy", "warn")
	runOK(t, directory, "region", "set", "--book", "guide", "--kind", "footer", "--body", "teh cat")

	runOK(t, directory, "config", "set", "grammar.policy", "block")
	code, _, stderr = run(t, directory, "region", "notes", "--book", "guide",
		"--target", "node", "--id", chapter, "--body", "teh cat")
	if code == cli.ExitOK || !strings.Contains(stderr, `"body.grammar-blocked"`) {
		t.Fatalf("region notes was not gated: %s", stderr)
	}
}
