package grammar_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
)

func TestTermsRejectBuiltinMutation(t *testing.T) {
	directory := t.TempDir()
	for _, term := range []string{"pecia", "CLABOK"} {
		code, _, stderr := run(t, directory, "terms", "set", "--term", term,
			"--reason", "already provided", "--scope", "global")
		if code != cli.ExitReject || !strings.Contains(stderr, "terms.builtin") {
			t.Fatalf("%q: exit %d %s", term, code, stderr)
		}
		code, _, stderr = run(t, directory, "terms", "delete", "--term", term,
			"--reason", "already provided", "--scope", "global")
		if code != cli.ExitReject || !strings.Contains(stderr, "terms.builtin") {
			t.Fatalf("delete %q: exit %d %s", term, code, stderr)
		}
	}
	if _, err := os.Stat(filepath.Join(directory, "_CLABOK", "terms.txt")); !os.IsNotExist(err) {
		t.Fatalf("a rejected request wrote the term file: %v", err)
	}
}

func TestTermsListMarksTheBuiltinLayer(t *testing.T) {
	directory := t.TempDir()
	listed := dataOf(t, runOK(t, directory, "terms", "list", "--language", "en-US"))
	layers, _ := listed["layers"].([]any)
	if len(layers) == 0 {
		t.Fatalf("layers %v", listed["layers"])
	}
	first, _ := layers[0].(map[string]any)
	if first["scope"] != "builtin" || first["readOnly"] != true {
		t.Fatalf("first layer %v", first)
	}
	text := runOK(t, directory, "terms", "list", "--language", "en-US", "--text")
	if !strings.Contains(text, "builtin") || !strings.Contains(text, "read-only") {
		t.Fatalf("text output\n%s", text)
	}
}
