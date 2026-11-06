// Package grammar gates prose writes with a LanguageTool check and owns the term list.
//
// A check is a pure read: the client sends the text to the configured endpoint and maps the
// LanguageTool matches to findings. The gate applies the configured policy; the writer decides what
// a finding means. Nothing here writes a body or a container.
package grammar

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

// DefaultEndpoint is used when the configuration leaves the endpoint empty.
const DefaultEndpoint = "http://127.0.0.1:8081/v2/check"

// Language is the fixed check language. The endpoint is the only grammar config key.
const Language = "en-US"

// ErrUnavailable reports a server that did not answer with a usable check result.
var ErrUnavailable = errors.New("grammar server unavailable")

// Policy is the gate decision for a finding.
type Policy string

const (
	// PolicyBlock refuses the write.
	PolicyBlock Policy = "block"
	// PolicyWarn writes and reports.
	PolicyWarn Policy = "warn"
)

// ParsePolicy maps the config value to a policy. There is no "off": the check always runs, and a
// server that cannot be started degrades to a skip.
func ParsePolicy(raw string) Policy {
	if Policy(raw) == PolicyWarn {
		return PolicyWarn
	}
	return PolicyBlock
}

// Finding is one grammar finding.
type Finding struct {
	Index        int      `json:"index"`
	Block        model.ID `json:"block,omitempty"`
	Line         int      `json:"line"`
	Column       int      `json:"column"`
	Offset       int      `json:"offset"`
	Length       int      `json:"length"`
	Rule         string   `json:"rule"`
	Message      string   `json:"message"`
	Offending    string   `json:"offending,omitempty"`
	Replacements []string `json:"replacements,omitempty"`
}

// Report is the outcome of one check.
type Report struct {
	Policy   string    `json:"policy,omitempty"`
	Checked  bool      `json:"checked"`
	Degraded string    `json:"degraded,omitempty"`
	Findings []Finding `json:"findings,omitempty"`
}

// Blocked reports whether the policy refuses the write.
func (report Report) Blocked() bool {
	return report.Policy == string(PolicyBlock) && len(report.Findings) > 0
}

// Client talks to one LanguageTool endpoint.
type Client struct {
	Endpoint string
	HTTP     *http.Client
	Terms    []string
}

// NewClient returns a client with the default timeout. An empty endpoint uses the local default.
func NewClient(endpoint string, terms []string) *Client {
	if strings.TrimSpace(endpoint) == "" {
		endpoint = DefaultEndpoint
	}
	return &Client{
		Endpoint: strings.TrimSpace(endpoint),
		HTTP:     &http.Client{Timeout: 5 * time.Second},
		Terms:    terms,
	}
}

type matchesPayload struct {
	Matches []struct {
		Message string `json:"message"`
		Offset  int    `json:"offset"`
		Length  int    `json:"length"`
		Rule    struct {
			ID string `json:"id"`
		} `json:"rule"`
		Replacements []struct {
			Value string `json:"value"`
		} `json:"replacements"`
	} `json:"matches"`
}

// Check sends the text and returns the unsuppressed findings in server order.
func (client *Client) Check(ctx context.Context, text string) ([]Finding, error) {
	form := url.Values{}
	form.Set("text", text)
	form.Set("language", Language)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.Endpoint,
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, ErrUnavailable
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.httpClient().Do(request)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, ErrUnavailable
	}
	var payload matchesPayload
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&payload); err != nil {
		return nil, ErrUnavailable
	}
	findings := make([]Finding, 0, len(payload.Matches))
	for _, match := range payload.Matches {
		offending := segment(text, match.Offset, match.Length)
		replacements := make([]string, 0, len(match.Replacements))
		for _, replacement := range match.Replacements {
			replacements = append(replacements, replacement.Value)
		}
		if client.suppressed(offending, replacements) {
			continue
		}
		line, column := position(text, match.Offset)
		findings = append(findings, Finding{
			Line: line, Column: column, Offset: match.Offset, Length: match.Length,
			Rule: match.Rule.ID, Message: match.Message, Offending: offending,
			Replacements: replacements,
		})
	}
	for index := range findings {
		findings[index].Index = index + 1
	}
	return findings, nil
}

func (client *Client) httpClient() *http.Client {
	if client.HTTP != nil {
		return client.HTTP
	}
	return http.DefaultClient
}

func (client *Client) suppressed(offending string, replacements []string) bool {
	lowered := strings.ToLower(offending)
	for _, term := range client.Terms {
		term = strings.ToLower(strings.TrimSpace(term))
		if term == "" {
			continue
		}
		if strings.Contains(lowered, term) {
			return true
		}
		for _, replacement := range replacements {
			if strings.Contains(strings.ToLower(replacement), term) {
				return true
			}
		}
	}
	return false
}

// Gate applies the policy to a check.
type Gate struct {
	Client *Client
	Policy Policy
	// Start starts the shared server on demand. A nil Start uses Serve with the discovered jar.
	Start func(endpoint string) error
}

// NewGate returns a gate for the configured endpoint and policy.
func NewGate(endpoint string, policy Policy, terms []string) *Gate {
	return &Gate{Client: NewClient(endpoint, terms), Policy: policy}
}

// Evaluate runs the check. When the endpoint is unreachable the gate starts the shared server and
// checks once more; a server that cannot be started degrades to a one-line skip report. There is no
// silent skip: every prose write passes through this gate.
func (gate *Gate) Evaluate(ctx context.Context, block model.ID, text string) Report {
	report := Report{Policy: string(PolicyBlock)}
	if gate == nil || gate.Client == nil {
		return report
	}
	report.Policy = string(gate.Policy)
	findings, err := gate.Client.Check(ctx, text)
	if err != nil {
		if startErr := gate.start(); startErr != nil {
			report.Degraded = fmt.Sprintf("grammar check skipped: %v", startErr)
			return report
		}
		findings, err = gate.Client.Check(ctx, text)
	}
	if err != nil {
		report.Degraded = fmt.Sprintf("grammar check skipped: %v", err)
		return report
	}
	for index := range findings {
		findings[index].Block = block
	}
	report.Checked = true
	report.Findings = findings
	return report
}

func (gate *Gate) start() error {
	if gate.Start != nil {
		return gate.Start(gate.Client.Endpoint)
	}
	_, err := Serve(gate.Client.Endpoint, "")
	return err
}

func segment(text string, offset, length int) string {
	if offset < 0 {
		offset = 0
	}
	if offset > len(text) {
		offset = len(text)
	}
	end := offset + length
	if end < offset {
		end = offset
	}
	if end > len(text) {
		end = len(text)
	}
	return text[offset:end]
}

func position(text string, offset int) (int, int) {
	if offset < 0 {
		offset = 0
	}
	if offset > len(text) {
		offset = len(text)
	}
	line, column := 1, 1
	for index := 0; index < offset; index++ {
		if text[index] == '\n' {
			line++
			column = 1
			continue
		}
		column++
	}
	return line, column
}

// ReadTerms reads the store-level term list. A missing file is an empty list.
func ReadTerms(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	terms := make([]string, 0)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			terms = append(terms, line)
		}
	}
	return terms
}
