package editorial

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/grammar"
)

// checkPayload runs grammar check in a registry that carries the grammar commands, which the shared
// editorial registry does not.
func checkPayload(t *testing.T, directory string, args ...string) map[string]any {
	t.Helper()
	registry := newRegistry()
	grammar.Register(registry)
	var stdout, stderr strings.Builder
	code := cli.Run(registry, append([]string{"--project", directory}, args...), &stdout, &stderr)
	if code != cli.ExitOK {
		t.Fatalf("%v: exit %d\n%s%s", args, code, stdout.String(), stderr.String())
	}
	return dataOf(t, stdout.String())
}

// TestBodySetAcceptsEntryFormat is the host acceptance for FR-0015: a body in the documented entry
// format cites commands as inline code, the server reports a real whitespace finding on the list
// marker, and the write under `grammar.policy: block` must still succeed because the finding sits on
// masked bytes.
func TestBodySetAcceptsEntryFormat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/languages" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `[{"code":"en-US","longCode":"en-US","name":"English (US)"}]`)
			return
		}
		_ = r.ParseForm()
		matches := make([]string, 0, 1)
		if at := strings.Index(r.Form.Get("text"), "  Status: open"); at >= 0 {
			matches = append(matches, fmt.Sprintf(
				`{"message":"rule","offset":%d,"length":2,"rule":{"id":"WHITESPACE_RULE"},`+
					`"replacements":[{"value":" "}]}`, at))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"matches":[%s]}`, strings.Join(matches, ","))
	}))
	defer server.Close()

	directory := t.TempDir()
	runOK(t, directory, "config", "set", "grammar.endpoint", server.URL)
	runOK(t, directory, "config", "set", "grammar.policy", "block")
	runOK(t, directory, "book", "new", "--library", "--id", "library", "--title", "CLABOK")
	runOK(t, directory, "book", "new", "--title", "Guide")
	runOK(t, directory, "book", "open", "guide")
	chapter := dataOf(t, runOK(t, directory, "add", "--book", "guide", "--title", "Chapter"))["target"].(string)

	entry := "#### FR-0001 Title\n- Status: open\n- Actual: the gate `pecia body set` refuses.\n"
	runOK(t, directory, "body", "set", "--book", "guide", "0", "--parent", chapter, "--body", entry)

	checked := checkPayload(t, directory, "grammar", "check", "--raw", entry)
	if count, _ := checked["count"].(float64); count != 0 {
		t.Fatalf("the entry body must not produce findings: %v", checked)
	}
	masked, _ := checked["masked"].([]any)
	if len(masked) != 1 {
		t.Fatalf("masked attribution %v", checked["masked"])
	}
	entryMasked, _ := masked[0].(map[string]any)
	if entryMasked["rule"] != "WHITESPACE_RULE" || entryMasked["count"].(float64) != 1 {
		t.Fatalf("masked entry %v", entryMasked)
	}
	if reason, _ := entryMasked["reason"].(string); reason == "" {
		t.Fatalf("a masked finding needs a reason: %v", entryMasked)
	}
}
