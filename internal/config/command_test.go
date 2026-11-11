package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
)

type commandEnvelope struct {
	OK      bool            `json:"ok"`
	Data    json.RawMessage `json:"data"`
	Error   *commandError   `json:"error"`
	Command string          `json:"command"`
}

type commandError struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	Details json.RawMessage `json:"details"`
}

func runCommand(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	registry := cli.NewRegistry()
	Register(registry)
	var stdout, stderr strings.Builder
	code := cli.Run(registry, args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func parseEnvelope(t *testing.T, text string) commandEnvelope {
	t.Helper()
	var payload commandEnvelope
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("not a JSON envelope: %v\n%s", err, text)
	}
	return payload
}

func TestSetCreatesFile(t *testing.T) {
	project := t.TempDir()
	t.Chdir(project)
	code, stdout, stderr := runCommand(t, "config", "set", "store.root", "data")
	if code != cli.ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	payload := parseEnvelope(t, stdout)
	if !payload.OK || payload.Command != "config set" {
		t.Fatalf("unexpected envelope %+v", payload)
	}
	var change struct {
		Changed bool   `json:"changed"`
		DryRun  bool   `json:"dryRun"`
		Key     string `json:"key"`
	}
	if err := json.Unmarshal(payload.Data, &change); err != nil {
		t.Fatal(err)
	}
	if !change.Changed || change.DryRun || change.Key != "store.root" {
		t.Fatalf("unexpected change %+v", change)
	}
	data, err := os.ReadFile(filepath.Join(project, FileName))
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if document["version"] != float64(Version) {
		t.Fatalf("version %v", document["version"])
	}
	store, ok := document["store"].(map[string]any)
	if !ok || store["root"] != "data" {
		t.Fatalf("store %+v", document["store"])
	}
}

func TestSetDryRunLeavesFileUntouched(t *testing.T) {
	project := t.TempDir()
	t.Chdir(project)
	path := writeConfigDocument(t, project, `{"version":1,"store":{"root":"data"}}`)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runCommand(t, "config", "set", "store.root", "other", "--dry-run")
	if code != cli.ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("dry run wrote the file:\n%s", after)
	}
	var change struct {
		Changed bool `json:"changed"`
		DryRun  bool `json:"dryRun"`
	}
	payload := parseEnvelope(t, stdout)
	if err := json.Unmarshal(payload.Data, &change); err != nil {
		t.Fatal(err)
	}
	if !change.DryRun || !change.Changed {
		t.Fatalf("unexpected receipt %+v", change)
	}
}

func TestGetReportsSource(t *testing.T) {
	project := t.TempDir()
	t.Chdir(project)
	writeConfigDocument(t, project, `{"version":1,"store":{"root":"data"}}`)

	code, stdout, stderr := runCommand(t, "config", "get", "store.root")
	if code != cli.ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var stored struct {
		Value  string `json:"value"`
		Source string `json:"source"`
	}
	if err := json.Unmarshal(parseEnvelope(t, stdout).Data, &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Source != "file" || stored.Value != filepath.Join(project, "data") {
		t.Fatalf("unexpected value %+v", stored)
	}

	code, stdout, _ = runCommand(t, "config", "get", "rollback.undoLimit")
	if code != cli.ExitOK {
		t.Fatalf("exit %d", code)
	}
	var fallback struct {
		Value  float64 `json:"value"`
		Source string  `json:"source"`
	}
	if err := json.Unmarshal(parseEnvelope(t, stdout).Data, &fallback); err != nil {
		t.Fatal(err)
	}
	if fallback.Source != "default" || fallback.Value != 10 {
		t.Fatalf("unexpected default %+v", fallback)
	}
}

func TestGetUnknownKey(t *testing.T) {
	project := t.TempDir()
	t.Chdir(project)
	code, _, stderr := runCommand(t, "config", "get", "store.bogus")
	if code != cli.ExitUsage {
		t.Fatalf("exit %d", code)
	}
	payload := parseEnvelope(t, stderr)
	if payload.Error == nil || payload.Error.Code != "usage.unknown-key" {
		t.Fatalf("unexpected error %+v", payload.Error)
	}
	if !strings.Contains(string(payload.Error.Details), "store.root") {
		t.Fatalf("missing key list: %s", payload.Error.Details)
	}
}

func TestUnsetRestoresDefault(t *testing.T) {
	project := t.TempDir()
	t.Chdir(project)
	path := writeConfigDocument(t, project,
		`{"version":1,"store":{"root":"data"},"temp":{"root":"cache"}}`)
	code, _, stderr := runCommand(t, "config", "unset", "store.root")
	if code != cli.ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if _, exists := document["store"]; exists {
		t.Fatalf("store section should be pruned: %s", data)
	}
	if _, exists := document["temp"]; !exists {
		t.Fatalf("unrelated key lost: %s", data)
	}
}

func TestUnsetDefaultIsNoOp(t *testing.T) {
	project := t.TempDir()
	t.Chdir(project)
	path := writeConfigDocument(t, project, `{"version":1,"temp":{"root":"cache"}}`)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	code, stdout, _ := runCommand(t, "config", "unset", "rollback.undoLimit")
	if code != cli.ExitOK {
		t.Fatalf("exit %d", code)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("no-op unset wrote the file")
	}
	var change struct {
		Changed bool `json:"changed"`
	}
	if err := json.Unmarshal(parseEnvelope(t, stdout).Data, &change); err != nil {
		t.Fatal(err)
	}
	if change.Changed {
		t.Fatal("expected changed to be false")
	}
}

func TestAsciiuiOverrideRoundTrip(t *testing.T) {
	project := t.TempDir()
	t.Chdir(project)
	if code, _, stderr := runCommand(t, "config", "set", "asciiui.types.panel", "ShopPanel"); code != cli.ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	code, stdout, _ := runCommand(t, "config", "get", "asciiui.types.panel")
	if code != cli.ExitOK {
		t.Fatalf("exit %d", code)
	}
	var stored struct {
		Value  string `json:"value"`
		Source string `json:"source"`
	}
	if err := json.Unmarshal(parseEnvelope(t, stdout).Data, &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Value != "ShopPanel" || stored.Source != "file" {
		t.Fatalf("unexpected override %+v", stored)
	}

	if code, _, _ := runCommand(t, "config", "set", "asciiui.types.panel", ""); code != cli.ExitOK {
		t.Fatalf("exit %d", code)
	}
	code, stdout, _ = runCommand(t, "config", "get", "asciiui.types.panel")
	if code != cli.ExitOK {
		t.Fatalf("exit %d", code)
	}
	if err := json.Unmarshal(parseEnvelope(t, stdout).Data, &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Value != "panel" || stored.Source != "default" {
		t.Fatalf("override not removed: %+v", stored)
	}
}

func TestInvalidValueKeepsFile(t *testing.T) {
	project := t.TempDir()
	t.Chdir(project)
	path := writeConfigDocument(t, project, `{"version":1,"store":{"root":"data"}}`)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runCommand(t, "config", "set", "rollback.undoLimit", "-1")
	if code != cli.ExitUsage {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("invalid set wrote the file")
	}
}

func TestShowWithoutFile(t *testing.T) {
	project := t.TempDir()
	t.Chdir(project)
	code, stdout, stderr := runCommand(t, "config", "show")
	if code != cli.ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var payload struct {
		File   string `json:"file"`
		Exists bool   `json:"exists"`
		Values struct {
			Store struct {
				Root string `json:"root"`
			} `json:"store"`
			Rollback struct {
				UndoLimit int `json:"undoLimit"`
			} `json:"rollback"`
			Concurrency struct {
				Mode string `json:"mode"`
			} `json:"concurrency"`
		} `json:"values"`
	}
	if err := json.Unmarshal(parseEnvelope(t, stdout).Data, &payload); err != nil {
		t.Fatal(err)
	}
	base := mustGetwd(t)
	if payload.File != "" || payload.Exists {
		t.Fatalf("unexpected file state %+v", payload)
	}
	if payload.Values.Store.Root != filepath.Join(base, "_CLABOK") {
		t.Fatalf("store root %q", payload.Values.Store.Root)
	}
	if payload.Values.Rollback.UndoLimit != 10 || payload.Values.Concurrency.Mode != "file" {
		t.Fatalf("unexpected defaults %+v", payload.Values)
	}
}

func TestShowHonoursProjectFlag(t *testing.T) {
	project := t.TempDir()
	writeConfigDocument(t, project, `{"version":1}`)
	other := t.TempDir()
	t.Chdir(other)
	code, stdout, stderr := runCommand(t, "config", "show", "--project", project)
	if code != cli.ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var payload struct {
		File       string `json:"file"`
		ProjectDir string `json:"projectDir"`
	}
	if err := json.Unmarshal(parseEnvelope(t, stdout).Data, &payload); err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(payload.ProjectDir) != filepath.Clean(project) {
		t.Fatalf("project dir %q", payload.ProjectDir)
	}
	if filepath.Clean(payload.File) != filepath.Join(project, FileName) {
		t.Fatalf("file %q", payload.File)
	}
}

func TestPathWithoutFile(t *testing.T) {
	project := t.TempDir()
	t.Chdir(project)
	code, stdout, stderr := runCommand(t, "config", "path")
	if code != cli.ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var payload struct {
		File   string `json:"file"`
		Exists bool   `json:"exists"`
	}
	if err := json.Unmarshal(parseEnvelope(t, stdout).Data, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.File != "" || payload.Exists {
		t.Fatalf("unexpected path payload %+v", payload)
	}
}

func TestValidateReportsAllIssues(t *testing.T) {
	project := t.TempDir()
	t.Chdir(project)
	writeConfigDocument(t, project,
		`{"version":1,"concurrency":{"mode":"carrier"},"rollback":{"undoLimit":-1}}`)
	code, _, stderr := runCommand(t, "config", "validate")
	if code != cli.ExitFailure {
		t.Fatalf("exit %d", code)
	}
	payload := parseEnvelope(t, stderr)
	if payload.Error == nil || payload.Error.Code != "config.invalid" {
		t.Fatalf("unexpected error %+v", payload.Error)
	}
	var details struct {
		Issues []Issue `json:"issues"`
	}
	if err := json.Unmarshal(payload.Error.Details, &details); err != nil {
		t.Fatal(err)
	}
	if len(details.Issues) != 2 {
		t.Fatalf("issues %+v", details.Issues)
	}
}

func TestTextOutput(t *testing.T) {
	project := t.TempDir()
	t.Chdir(project)
	code, stdout, stderr := runCommand(t, "config", "show", "--text")
	if code != cli.ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, "store:") || strings.Contains(stdout, "{") {
		t.Fatalf("unexpected text output %q", stdout)
	}
}

func writeOpenState(t *testing.T, project, body string) {
	t.Helper()
	directory := filepath.Join(project, "_CLABOK")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "open.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestModeChangeBlockedByOpenBook(t *testing.T) {
	project := t.TempDir()
	t.Chdir(project)
	writeConfigDocument(t, project, `{"version":1,"concurrency":{"mode":"file"}}`)
	writeOpenState(t, project, `{"version":1,"books":[{"id":"x"}]}`)

	code, _, stderr := runCommand(t, "config", "set", "concurrency.mode", "daemon")
	if code != cli.ExitReject {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if !strings.Contains(stderr, "config.open-books") {
		t.Fatalf("unexpected reject %s", stderr)
	}

	if err := os.Remove(filepath.Join(project, "_CLABOK", "open.json")); err != nil {
		t.Fatal(err)
	}
	code, _, stderr = runCommand(t, "config", "set", "concurrency.mode", "daemon")
	if code != cli.ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
}

func TestModeUnsetBlockedByOpenBook(t *testing.T) {
	project := t.TempDir()
	t.Chdir(project)
	writeConfigDocument(t, project, `{"version":1,"concurrency":{"mode":"daemon"}}`)
	writeOpenState(t, project, `{"version":1,"books":[{"id":"x"}]}`)

	code, _, stderr := runCommand(t, "config", "unset", "concurrency.mode")
	if code != cli.ExitReject {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if !strings.Contains(stderr, "config.open-books") {
		t.Fatalf("unexpected reject %s", stderr)
	}
}

func TestGrammarTimeoutKey(t *testing.T) {
	project := t.TempDir()
	t.Chdir(project)

	code, _, stderr := runCommand(t, "config", "set", "grammar.timeout", "2s")
	if code != cli.ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	code, stdout, stderr := runCommand(t, "config", "get", "grammar.timeout")
	if code != cli.ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var got struct {
		Value  any    `json:"value"`
		Source string `json:"source"`
	}
	if err := json.Unmarshal(parseEnvelope(t, stdout).Data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Value != "2s" || got.Source != "file" {
		t.Fatalf("timeout %+v", got)
	}

	code, _, stderr = runCommand(t, "config", "set", "grammar.timeout", "0s")
	if code == cli.ExitOK || !strings.Contains(stderr, "grammar.timeout") {
		t.Fatalf("a zero timeout must be rejected: %s", stderr)
	}
}

func TestGrammarLanguageKey(t *testing.T) {
	project := t.TempDir()
	t.Chdir(project)

	code, stdout, stderr := runCommand(t, "config", "get", "grammar.language")
	if code != cli.ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var got struct {
		Value  any    `json:"value"`
		Source string `json:"source"`
	}
	if err := json.Unmarshal(parseEnvelope(t, stdout).Data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Value != "en-US" || got.Source != "default" {
		t.Fatalf("default language %+v", got)
	}

	if code, _, stderr := runCommand(t, "config", "set", "grammar.language", "de-DE"); code != cli.ExitOK {
		t.Fatalf("set exit %d: %s", code, stderr)
	}
	code, stdout, stderr = runCommand(t, "config", "get", "grammar.language")
	if code != cli.ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if err := json.Unmarshal(parseEnvelope(t, stdout).Data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Value != "de-DE" || got.Source != "file" {
		t.Fatalf("language %+v", got)
	}

	code, _, stderr = runCommand(t, "config", "set", "grammar.language", "de_DE")
	if code == cli.ExitOK || !strings.Contains(stderr, "grammar.language") {
		t.Fatalf("an invalid tag must be rejected: %s", stderr)
	}
}
