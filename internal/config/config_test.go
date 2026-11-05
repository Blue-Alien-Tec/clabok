package config

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
)

func testEnv() *cli.Env {
	return &cli.Env{Out: io.Discard, Err: io.Discard}
}

func writeConfigDocument(t *testing.T, directory, body string) string {
	t.Helper()
	path := filepath.Join(directory, FileName)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func mustGetwd(t *testing.T) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return directory
}

func mustLoad(t *testing.T, environment *cli.Env) *Values {
	t.Helper()
	values, err := Load(environment)
	if err != nil {
		t.Fatalf("%s: %s", err.Code, err.Message)
	}
	return values
}

func TestDefaultsWithoutFile(t *testing.T) {
	directory := t.TempDir()
	t.Chdir(directory)
	values := mustLoad(t, testEnv())
	if values.Exists || values.File != "" {
		t.Fatalf("expected no file, got %+v", values)
	}
	base := mustGetwd(t)
	if values.StoreRoot != filepath.Join(base, "_CLABOK") {
		t.Fatalf("store root %q", values.StoreRoot)
	}
	if values.TempRoot != filepath.Join(base, ".temp") {
		t.Fatalf("temp root %q", values.TempRoot)
	}
	if values.DocsRoot != filepath.Join(base, "Docs") {
		t.Fatalf("docs root %q", values.DocsRoot)
	}
	if values.UndoLimit != 10 || values.BookPointLimit != 3 {
		t.Fatalf("limits %+v", values)
	}
	if values.ConcurrencyMode != "file" {
		t.Fatalf("mode %q", values.ConcurrencyMode)
	}
}

func TestDiscoveryUpward(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	deep := filepath.Join(project, "a", "b")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	writeConfigDocument(t, project, `{"version":1}`)
	t.Chdir(deep)
	values := mustLoad(t, testEnv())
	if filepath.Clean(values.ProjectDir) != filepath.Clean(project) {
		t.Fatalf("project dir %q", values.ProjectDir)
	}
	if !values.Exists {
		t.Fatal("expected the discovered file to exist")
	}
}

func TestRelativeRootsResolveAgainstConfigFile(t *testing.T) {
	project := t.TempDir()
	sub := filepath.Join(project, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	writeConfigDocument(t, project, `{"version":1,"store":{"root":"data/store"},"temp":{"root":"tmp"}}`)
	t.Chdir(sub)
	values := mustLoad(t, testEnv())
	if values.StoreRoot != filepath.Join(project, "data", "store") {
		t.Fatalf("store root %q", values.StoreRoot)
	}
	if values.TempRoot != filepath.Join(project, "tmp") {
		t.Fatalf("temp root %q", values.TempRoot)
	}
}

func TestFlagOverridesFileAndDefault(t *testing.T) {
	project := t.TempDir()
	writeConfigDocument(t, project, `{"version":1,"store":{"root":"from-file"}}`)
	t.Chdir(project)
	override := filepath.Join(project, "from-flag")
	environment := testEnv()
	environment.Globals.StoreRoot = override
	values := mustLoad(t, environment)
	if values.StoreRoot != override {
		t.Fatalf("store root %q", values.StoreRoot)
	}
	if values.DocsRoot != filepath.Join(project, "Docs") {
		t.Fatalf("default lost: %q", values.DocsRoot)
	}
}

func TestExplicitConfigMissing(t *testing.T) {
	environment := testEnv()
	environment.Globals.Config = filepath.Join(t.TempDir(), "missing.json")
	_, err := Load(environment)
	if err == nil || err.Code != "config.not-found" {
		t.Fatalf("expected config.not-found, got %v", err)
	}
}

func TestStrictDecoding(t *testing.T) {
	cases := []struct {
		body string
		code string
	}{
		{`{"version":1,"bogus":true}`, "config.unknown-key"},
		{`{"version":1,"version":1}`, "config.duplicate-key"},
		{`{}`, "config.version-missing"},
		{`{"version":2}`, "config.version-unsupported"},
		{`{"version":1,"store":{"root":""}}`, "config.invalid"},
		{`{"version":1,"concurrency":{"mode":"carrier"},"rollback":{"undoLimit":-1}}`, "config.invalid"},
		{`{"version":1,"grammar":{"endpoint":"not a url"}}`, "config.invalid"},
	}
	for _, testCase := range cases {
		project := t.TempDir()
		writeConfigDocument(t, project, testCase.body)
		t.Chdir(project)
		_, err := Load(testEnv())
		if err == nil {
			t.Fatalf("%s: expected an error", testCase.body)
		}
		if err.Code != testCase.code {
			t.Fatalf("%s: code %q, want %q", testCase.body, err.Code, testCase.code)
		}
	}
}

func TestValidationAggregatesIssues(t *testing.T) {
	project := t.TempDir()
	writeConfigDocument(t, project,
		`{"version":1,"concurrency":{"mode":"carrier"},"rollback":{"undoLimit":-1}}`)
	t.Chdir(project)
	_, err := Load(testEnv())
	if err == nil {
		t.Fatal("expected an error")
	}
	payload, ok := err.Details.(issuesPayload)
	if !ok {
		t.Fatalf("details %T", err.Details)
	}
	if len(payload.Issues) != 2 {
		t.Fatalf("issues %+v", payload.Issues)
	}
}

func TestEncodeRoundTrip(t *testing.T) {
	document, err := decode([]byte(`{"version":1,"asciiui":{"types":{"panel":"ShopPanel"}},"store":{"root":"data"}}`))
	if err != nil {
		t.Fatal(err)
	}
	data, encodeErr := encode(document)
	if encodeErr != nil {
		t.Fatal(encodeErr)
	}
	text := string(data)
	if !strings.Contains(text, `"version": 1`) {
		t.Fatalf("missing version: %s", text)
	}
	if !strings.Contains(text, `"panel": "ShopPanel"`) {
		t.Fatalf("missing override: %s", text)
	}
	if !strings.HasSuffix(text, "\n") {
		t.Fatal("missing trailing newline")
	}
}

func TestEncodePrunesEmptySections(t *testing.T) {
	document, err := decode([]byte(`{"version":1,"store":{"root":"data"}}`))
	if err != nil {
		t.Fatal(err)
	}
	document.Store.Root = nil
	data, encodeErr := encode(document)
	if encodeErr != nil {
		t.Fatal(encodeErr)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, exists := decoded["store"]; exists {
		t.Fatalf("empty section not pruned: %s", data)
	}
}
