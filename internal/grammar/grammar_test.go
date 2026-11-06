package grammar

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"
)

const matchPayload = `{"matches":[{"message":"Possible typo","offset":4,"length":3,` +
	`"rule":{"id":"MORFOLOGIK_RULE_EN_US"},"replacements":[{"value":"the"}]}]}`

func matchServer(t *testing.T, calls *int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls != nil {
			*calls++
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		if language := r.Form.Get("language"); language != Language {
			t.Errorf("language %q", language)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, matchPayload)
	}))
}

func TestClientCheck(t *testing.T) {
	server := matchServer(t, nil)
	defer server.Close()

	findings, err := NewClient(server.URL, nil).Check(context.Background(), "one\nteh cat")
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

	findings, err := NewClient(server.URL, []string{"TEH"}).Check(context.Background(), "one\nteh cat")
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

	_, err := NewClient(url, nil).Check(context.Background(), "text")
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

	findings, err := NewClient(endpoint, nil).Check(context.Background(), "This are a test.")
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
	gate := NewGate(server.URL, PolicyWarn, nil)
	report := gate.Evaluate(context.Background(), 10, "teh cat")
	if !report.Checked || len(report.Findings) != 1 || report.Blocked() {
		t.Fatalf("warn report %+v", report)
	}
	if report.Findings[0].Block != 10 {
		t.Fatalf("finding block %+v", report.Findings[0])
	}

	gate.Policy = PolicyBlock
	report = gate.Evaluate(context.Background(), 10, "teh cat")
	if !report.Blocked() {
		t.Fatalf("block report %+v", report)
	}

	server.Close()
	started := false
	gate.Start = func(endpoint string) error {
		started = true
		return errors.New("no server")
	}
	report = gate.Evaluate(context.Background(), 10, "teh cat")
	if !started || report.Degraded == "" || len(report.Findings) != 0 || report.Blocked() {
		t.Fatalf("degraded report %+v (started %v)", report, started)
	}
}
