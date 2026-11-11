package grammar

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

const slowMatchPayload = `{"matches":[{"message":"Possible typo","offset":0,"length":3,` +
	`"rule":{"id":"MORFOLOGIK_RULE_EN_US"},"replacements":[{"value":"the"}]}]}`

func TestWatchdogAcceptsLateAnswerFromOneRequest(t *testing.T) {
	var checks, probes int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/languages" {
			atomic.AddInt32(&probes, 1)
			w.WriteHeader(http.StatusOK)
			return
		}
		atomic.AddInt32(&checks, 1)
		time.Sleep(90 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, slowMatchPayload)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	client.Timeout = 40 * time.Millisecond
	findings, err := checkDefault(t, client, "teh cat")
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings %+v", findings)
	}
	if got := atomic.LoadInt32(&checks); got != 1 {
		t.Fatalf("a late answer must come from the same request, got %d requests", got)
	}
	if atomic.LoadInt32(&probes) == 0 {
		t.Fatal("the watchdog must probe a running request")
	}
}

func TestWatchdogRearmsAfterOneFailedProbe(t *testing.T) {
	var probes int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/languages" {
			if atomic.AddInt32(&probes, 1) == 1 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		time.Sleep(90 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, slowMatchPayload)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	client.Timeout = 40 * time.Millisecond
	seedLanguages(t, server.URL, DefaultLanguage)
	gate := &Gate{Client: client, Policy: PolicyWarn}
	started := false
	gate.Start = func(string) error {
		started = true
		return errors.New("no restart expected")
	}
	report := gate.Evaluate(context.Background(), 0, "teh cat", DefaultLanguage)
	if started {
		t.Fatal("a single failed probe must not restart the server")
	}
	if !report.Checked || len(report.Findings) != 1 {
		t.Fatalf("report %+v", report)
	}
}

func TestWatchdogTwoDeadProbesTakeTheUnreachablePath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/languages" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		time.Sleep(500 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, slowMatchPayload)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	client.Timeout = 20 * time.Millisecond
	seedLanguages(t, server.URL, DefaultLanguage)
	gate := &Gate{Client: client, Policy: PolicyBlock}
	started := false
	gate.Start = func(string) error {
		started = true
		return errors.New("no server")
	}
	report := gate.Evaluate(context.Background(), 0, "teh cat", DefaultLanguage)
	if !started {
		t.Fatal("two dead probes must trigger the start attempt")
	}
	if report.Checked || report.Reason != ReasonStartFailed {
		t.Fatalf("report %+v", report)
	}
	if report.Blocked() {
		t.Fatal("an unchecked check must not block a write")
	}
	if len(report.Warnings()) != 1 {
		t.Fatalf("exactly one warning expected: %+v", report.Warnings())
	}
}

func TestWatchdogCapTimesOutAgainstLiveServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/languages" {
			w.WriteHeader(http.StatusOK)
			return
		}
		time.Sleep(500 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, slowMatchPayload)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	client.Timeout = 20 * time.Millisecond
	start := time.Now()
	_, err := checkDefault(t, client, "teh cat")
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("expected ErrTimeout, got %v", err)
	}
	if errors.Is(err, ErrUnreachable) {
		t.Fatal("a live server is a timeout, not an unreachable server")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("the cap must be bounded, took %s", elapsed)
	}
	if ReasonFor(err) != ReasonTimeout {
		t.Fatalf("reason %q", ReasonFor(err))
	}
}

func TestCheckTransportErrors(t *testing.T) {
	unreachable := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := unreachable.URL
	unreachable.Close()
	_, err := checkDefault(t, NewClient(url), "text")
	if !errors.Is(err, ErrUnreachable) || !errors.Is(err, ErrUnavailable) {
		t.Fatalf("connection refused: %v", err)
	}
	if ReasonFor(err) != ReasonUnreachable {
		t.Fatalf("reason %q", ReasonFor(err))
	}

	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer failing.Close()
	_, err = checkDefault(t, NewClient(failing.URL), "text")
	var response *ErrResponse
	if !errors.As(err, &response) || response.Status != http.StatusInternalServerError {
		t.Fatalf("expected ErrResponse, got %v", err)
	}
	if ReasonFor(err) != ReasonResponse {
		t.Fatalf("reason %q", ReasonFor(err))
	}

	garbled := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"matches":`)
	}))
	defer garbled.Close()
	_, err = checkDefault(t, NewClient(garbled.URL), "text")
	if !errors.As(err, &response) || ReasonFor(err) != ReasonResponse {
		t.Fatalf("expected ErrResponse for malformed JSON, got %v", err)
	}
}

func TestGateSkipIsTruthful(t *testing.T) {
	client := NewClient("http://127.0.0.1:1/v2/check")
	client.Timeout = 20 * time.Millisecond
	gate := &Gate{Client: client, Policy: PolicyBlock}
	gate.Start = func(string) error { return errors.New("no server") }
	report := gate.Evaluate(context.Background(), 0, "teh cat", DefaultLanguage)
	if report.Checked || !report.Unchecked() {
		t.Fatalf("report %+v", report)
	}
	if report.Reason != ReasonStartFailed || report.Degraded == "" {
		t.Fatalf("report %+v", report)
	}
	warnings := report.Warnings()
	if len(warnings) != 1 || warnings[0].Code != ReasonStartFailed || warnings[0].Message == "" {
		t.Fatalf("warnings %+v", warnings)
	}
	if report.Unchecked() != true || (Report{Checked: true}).Warnings() != nil {
		t.Fatal("a complete check carries no warning")
	}
}

func TestClientCapFollowsInterval(t *testing.T) {
	client := NewClient("")
	if client.Interval() != DefaultTimeout {
		t.Fatalf("default interval %s", client.Interval())
	}
	if client.Cap() != WatchdogIntervals*DefaultTimeout {
		t.Fatalf("default cap %s", client.Cap())
	}
	client.Timeout = time.Second
	if client.Cap() != 3*time.Second || client.Interval() != time.Second {
		t.Fatalf("interval %s cap %s", client.Interval(), client.Cap())
	}
}
