package grammar

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

const matchPayload = `{"matches":[{"message":"Possible typo","offset":4,"length":3,` +
	`"rule":{"id":"MORFOLOGIK_RULE_EN_US"},"replacements":[{"value":"the"}]}]}`

// TestMain isolates the grammar state directory so caches never leak between tests.
func TestMain(m *testing.M) {
	directory, err := os.MkdirTemp("", "clabok-grammar-test-")
	if err != nil {
		panic(err)
	}
	os.Setenv("CLABOK_GRAMMAR_STATE_DIR", directory)
	code := m.Run()
	os.RemoveAll(directory)
	os.Exit(code)
}

// seedLanguages writes a fresh language cache so a gate test needs no language endpoint.
func seedLanguages(t *testing.T, endpoint string, codes ...string) {
	t.Helper()
	languages := make([]Language, 0, len(codes))
	for _, code := range codes {
		languages = append(languages, Language{Code: code, Name: code})
	}
	list := LanguageList{Endpoint: endpoint, Fetched: time.Now().UTC(), Languages: languages}
	data, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stateDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(languageCachePath(), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// checkDefault runs a check in the built-in language and returns its findings.
func checkDefault(t *testing.T, client *Client, text string) ([]Finding, error) {
	t.Helper()
	result, err := client.Check(context.Background(), text, DefaultLanguage)
	return result.Findings, err
}

func matchServer(t *testing.T, calls *int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls != nil {
			*calls++
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		if language := r.Form.Get("language"); language != DefaultLanguage {
			t.Errorf("language %q", language)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, matchPayload)
	}))
}

func TestClientCheck(t *testing.T) {
	server := matchServer(t, nil)
	defer server.Close()

	findings, err := checkDefault(t, NewClient(server.URL, nil), "one\nteh cat")
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings %+v", findings)
	}
	finding := findings[0]
	if finding.Index != 1 || finding.Line != 2 || finding.Column != 1 {
		t.Fatalf("position %+v", finding)
	}
	if finding.Rule != "MORFOLOGIK_RULE_EN_US" || finding.Offending != "teh" ||
		len(finding.Replacements) != 1 || finding.Replacements[0] != "the" {
		t.Fatalf("finding %+v", finding)
	}
}

func TestTermSuppression(t *testing.T) {
	server := matchServer(t, nil)
	defer server.Close()

	findings, err := checkDefault(t, NewClient(server.URL, []string{"TEH"}), "one\nteh cat")
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("a term must suppress the finding: %+v", findings)
	}
}

func TestUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := server.URL
	server.Close()

	_, err := checkDefault(t, NewClient(url, nil), "text")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
}

func TestParseEndpoint(t *testing.T) {
	base, port, err := parseEndpoint("")
	if err != nil || base != "http://127.0.0.1:8081" || port != 8081 {
		t.Fatalf("default: %q %d %v", base, port, err)
	}
	base, port, err = parseEndpoint("http://localhost:9999/v2/check")
	if err != nil || base != "http://localhost:9999" || port != 9999 {
		t.Fatalf("explicit: %q %d %v", base, port, err)
	}
	if _, _, err := parseEndpoint("http://example.com/v2/check"); err == nil {
		t.Fatal("a remote endpoint must not be started")
	}
}

func TestLanguageToolLive(t *testing.T) {
	if testing.Short() {
		t.Skip("live integration test")
	}
	if _, err := exec.LookPath("java"); err != nil {
		t.Skip("java is not available")
	}
	jar := resolveJar("")
	if jar == "" {
		t.Skip("no LanguageTool jar found")
	}
	t.Setenv("CLABOK_GRAMMAR_STATE_DIR", t.TempDir())
	port := freePort(t)
	endpoint := fmt.Sprintf("http://127.0.0.1:%d/v2/check", port)

	info, err := Serve(endpoint, jar)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, stopErr := Stop(endpoint); stopErr != nil {
			t.Logf("stop: %v", stopErr)
		}
	}()
	if !info.Running || info.PID == 0 {
		t.Fatalf("server info %+v", info)
	}

	findings, err := checkDefault(t, NewClient(endpoint, nil), "This are a test.")
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) == 0 {
		t.Fatal("the live server returned no findings for a known error")
	}
	t.Logf("live findings: %+v", findings)
}

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}

func TestGatePolicies(t *testing.T) {
	server := matchServer(t, nil)
	seedLanguages(t, server.URL, DefaultLanguage, "de-DE")
	gate := NewGate(server.URL, PolicyWarn, nil)
	report := gate.Evaluate(context.Background(), 10, "teh cat", DefaultLanguage)
	if !report.Checked || len(report.Findings) != 1 || report.Blocked() {
		t.Fatalf("warn report %+v", report)
	}
	if report.Findings[0].Block != 10 {
		t.Fatalf("finding block %+v", report.Findings[0])
	}
	if report.Language != DefaultLanguage {
		t.Fatalf("report language %q", report.Language)
	}

	gate.Policy = PolicyBlock
	report = gate.Evaluate(context.Background(), 10, "teh cat", DefaultLanguage)
	if !report.Blocked() {
		t.Fatalf("block report %+v", report)
	}

	server.Close()
	started := false
	gate.Start = func(endpoint string) error {
		started = true
		return errors.New("no server")
	}
	report = gate.Evaluate(context.Background(), 10, "teh cat", DefaultLanguage)
	if !started || report.Degraded == "" || len(report.Findings) != 0 || report.Blocked() {
		t.Fatalf("degraded report %+v (started %v)", report, started)
	}
}

func TestPrepareText(t *testing.T) {
	input := "Prose `--id` and text.\n```text\n./... --id\n```\nAfter."
	prepared := prepareText(input)
	if strings.Contains(prepared.text, "--id") || strings.Contains(prepared.text, "./...") {
		t.Fatalf("code survived the prose view: %q", prepared.text)
	}
	if !strings.Contains(prepared.text, "Prose") || !strings.Contains(prepared.text, "After.") {
		t.Fatalf("prose was removed: %q", prepared.text)
	}
	codeAt := strings.Index(prepared.text, "code")
	if codeAt < 0 || !prepared.inCode(prepared.original(codeAt)) {
		t.Fatalf("the code span is not mapped: %q", prepared.text)
	}
	if prepared.original(len(prepared.text)) != len(input) {
		t.Fatalf("the sentinel does not point at the body end")
	}
}
