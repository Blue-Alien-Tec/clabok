package editorial

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/apply"
	"github.com/Blue-Alien-Tec/clabok/internal/body"
	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/config"
	"github.com/Blue-Alien-Tec/clabok/internal/coordinator"
	"github.com/Blue-Alien-Tec/clabok/internal/library"
	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/render"
	"github.com/Blue-Alien-Tec/clabok/internal/search"
	"github.com/Blue-Alien-Tec/clabok/internal/structure"
	"github.com/Blue-Alien-Tec/clabok/internal/temp"
)

func newRegistry() *cli.Registry {
	registry := cli.NewRegistry()
	config.Register(registry)
	coordinator.Register(registry)
	lifecycle.Register(registry)
	structure.Register(registry)
	Register(registry)
	apply.Register(registry)
	body.Register(registry)
	search.Register(registry)
	temp.Register(registry)
	render.Register(registry, nil)
	library.Register(registry)
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

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// quietGrammar points the project at an empty fake server so tests that exercise editorial content
// do not start the real LanguageTool. The production gate always checks; the fake keeps the suite
// offline and fast.
func quietGrammar(t *testing.T, directory string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"matches":[]}`)
	}))
	t.Cleanup(server.Close)
	runOK(t, directory, "config", "set", "grammar.endpoint", server.URL)
}

func TestEditorialCommandsEndToEnd(t *testing.T) {
	directory := t.TempDir()
	quietGrammar(t, directory)
	runOK(t, directory, "book", "new", "--library", "--id", "library", "--title", "CLABOK")
	runOK(t, directory, "book", "new", "--title", "Guide")
	runOK(t, directory, "book", "open", "guide")
	chapter := dataOf(t, runOK(t, directory, "add", "--book", "guide", "--title", "Chapter"))["target"].(string)

	prose := dataOf(t, runOK(t, directory, "prose", "set", "--book", "guide", "--node", chapter,
		"--prose", "Short summary"))
	if block, _ := prose["block"].(string); block == "" {
		t.Fatalf("prose did not report its block: %v", prose)
	}
	runOK(t, directory, "region", "set", "--book", "guide", "--kind", "footer", "--body", "Footer text")
	runOK(t, directory, "region", "notes", "--book", "guide", "--target", "node", "--id", chapter,
		"--body", "Note text")
	runOK(t, directory, "book", "close", "guide")
	runOK(t, directory, "sync", "--write")

	readme := readFile(t, filepath.Join(directory, "Docs", "guide", "README.md"))
	for _, expected := range []string{
		"| [Chapter](#chapter) | Short summary |",
		"Footer text",
		"Note text",
	} {
		if !strings.Contains(readme, expected) {
			t.Fatalf("missing %q:\n%s", expected, readme)
		}
	}
	runOK(t, directory, "sync", "--check")
}

func TestEditorialDryRunWritesNothing(t *testing.T) {
	directory := t.TempDir()
	quietGrammar(t, directory)
	runOK(t, directory, "book", "new", "--library", "--id", "library", "--title", "CLABOK")
	runOK(t, directory, "book", "new", "--title", "Guide")
	runOK(t, directory, "book", "open", "guide")
	chapter := dataOf(t, runOK(t, directory, "add", "--book", "guide", "--title", "Chapter"))["target"].(string)
	before := readFile(t, filepath.Join(directory, "_CLABOK", "guide", "manifest.json"))

	runOK(t, directory, "prose", "set", "--book", "guide", "--node", chapter, "--prose", "Dry", "--dry-run")
	runOK(t, directory, "region", "set", "--book", "guide", "--kind", "footer", "--body", "Dry", "--dry-run")
	runOK(t, directory, "region", "notes", "--book", "guide", "--target", "node", "--id", chapter,
		"--body", "Dry", "--dry-run")

	after := readFile(t, filepath.Join(directory, "_CLABOK", "guide", "manifest.json"))
	if before != after {
		t.Fatal("a dry run wrote the manifest")
	}
}
