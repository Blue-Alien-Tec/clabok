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
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

// DefaultEndpoint is used when the configuration leaves the endpoint empty.
const DefaultEndpoint = "http://127.0.0.1:8081/v2/check"

// DefaultLanguage is the built-in check language and the default of grammar.language. A book's
// declared language, then grammar.language, override it.
const DefaultLanguage = "en-US"

// DefaultTimeout is the default watchdog interval. The check cap is WatchdogIntervals times the
// interval; a check that outlives the cap is reported as grammar.timeout.
const DefaultTimeout = 5 * time.Second

// WatchdogIntervals is the number of consecutive alive intervals a request may run before the
// watchdog cancels it. The cap is fixed so the timeout stays a single configuration key.
const WatchdogIntervals = 3

// Stable reasons carried by a skipped check.
const (
	ReasonTimeout     = "grammar.timeout"
	ReasonUnreachable = "grammar.unreachable"
	ReasonResponse    = "grammar.response"
	ReasonStartFailed = "grammar.start-failed"
	// ReasonLanguageUnsupported reports a declared language the server does not offer, or the
	// ad-hoc-only auto value on a write path. The check is never sent in a fallback language.
	ReasonLanguageUnsupported = "grammar.language-unsupported"
)

// ErrUnavailable is the compatibility sentinel for every transport failure. New code discriminates
// with ErrTimeout, ErrUnreachable and ErrResponse; the gate never uses the sentinel itself.
var ErrUnavailable = errors.New("grammar server unavailable")

type sentinelError struct{ message string }

func (sentinel *sentinelError) Error() string { return sentinel.message }

func (sentinel *sentinelError) Unwrap() error { return ErrUnavailable }

// ErrTimeout reports that the watchdog cap was reached while the server stayed alive on every probe.
var ErrTimeout error = &sentinelError{message: "grammar check timed out"}

// ErrUnreachable reports that no connection could be established.
var ErrUnreachable error = &sentinelError{message: "grammar server unreachable"}

// ErrResponse reports a server that answered without a usable check result.
type ErrResponse struct {
	Status int
	Detail string
}

func (response *ErrResponse) Error() string {
	if response.Detail != "" {
		return fmt.Sprintf("grammar server error (HTTP %d): %s", response.Status, response.Detail)
	}
	return fmt.Sprintf("grammar server error (HTTP %d)", response.Status)
}

func (response *ErrResponse) Unwrap() error { return ErrUnavailable }

// ReasonFor maps a transport failure to its stable grammar.* reason.
func ReasonFor(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrTimeout):
		return ReasonTimeout
	case errors.Is(err, ErrUnreachable):
		return ReasonUnreachable
	}
	var response *ErrResponse
	if errors.As(err, &response) {
		return ReasonResponse
	}
	return ReasonUnreachable
}

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
	Policy     string          `json:"policy,omitempty"`
	Checked    bool            `json:"checked"`
	Reason     string          `json:"reason,omitempty"`
	Language   string          `json:"language,omitempty"`
	Degraded   string          `json:"degraded,omitempty"`
	Findings   []Finding       `json:"findings,omitempty"`
	Suppressed []Suppression   `json:"suppressed,omitempty"`
	Masked     []MaskedFinding `json:"masked,omitempty"`
}

// Suppression records one finding a term layer removed, so suppression is never silent.
type Suppression struct {
	Term   string   `json:"term"`
	Layer  string   `json:"suppressedBy"`
	AlsoIn []string `json:"alsoIn,omitempty"`
	Match  string   `json:"match"`
	Field  string   `json:"field"`
	Offset int      `json:"offset"`
	Length int      `json:"length"`
}

// MaskedFinding counts the findings one documented false-positive rule removed. The rule set is
// language independent in shape: an entry is keyed by the rule the server emitted, so a new artefact
// is one row and never a branch on the check language.
type MaskedFinding struct {
	Rule   string `json:"rule"`
	Count  int    `json:"count"`
	Reason string `json:"reason"`
}

// Blocked reports whether the policy refuses the write. Only findings block a write; an unchecked
// check is a skip, never a refusal.
func (report Report) Blocked() bool {
	return report.Policy == string(PolicyBlock) && len(report.Findings) > 0
}

// Unchecked reports whether the check did not produce a complete result. A skipped check always
// carries a Reason.
func (report Report) Unchecked() bool { return !report.Checked }

// Warning is one non-blocking condition of a command payload.
type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Warnings returns the single warning of an unchecked check, or nothing for a complete check.
func (report Report) Warnings() []Warning {
	if report.Checked || report.Reason == "" {
		return nil
	}
	return []Warning{{Code: report.Reason, Message: report.Degraded}}
}

// Client talks to one LanguageTool endpoint.
type Client struct {
	Endpoint string
	HTTP     *http.Client
	// Terms is the effective term set of the last evaluation. A gate replaces it per check.
	Terms TermSet
	// Timeout is the watchdog interval. A non-positive value uses DefaultTimeout.
	Timeout time.Duration
}

// NewClient returns a client with the default watchdog interval. An empty endpoint uses the local
// default. The request carries no hard deadline: the watchdog decides when a check is given up.
func NewClient(endpoint string) *Client {
	return NewClientTimeout(endpoint, 0)
}

// NewClientTimeout returns a client with an explicit watchdog interval. A non-positive timeout uses
// DefaultTimeout.
func NewClientTimeout(endpoint string, timeout time.Duration) *Client {
	if strings.TrimSpace(endpoint) == "" {
		endpoint = DefaultEndpoint
	}
	dialer := &net.Dialer{Timeout: 2 * time.Second}
	client := &Client{
		Endpoint: strings.TrimSpace(endpoint),
		HTTP: &http.Client{Transport: &http.Transport{
			Proxy:       http.ProxyFromEnvironment,
			DialContext: dialer.DialContext,
		}},
		Timeout: timeout,
	}
	return client
}

// Interval returns the effective watchdog interval.
func (client *Client) Interval() time.Duration {
	if client.Timeout > 0 {
		return client.Timeout
	}
	return DefaultTimeout
}

// Cap returns the effective watchdog cap of one request.
func (client *Client) Cap() time.Duration { return WatchdogIntervals * client.Interval() }

// Watchdog describes the effective interval and its cap for a configured timeout.
func Watchdog(timeout time.Duration) (time.Duration, time.Duration) {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return timeout, WatchdogIntervals * timeout
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
	Language struct {
		Code             string `json:"code"`
		DetectedLanguage struct {
			Code string `json:"code"`
		} `json:"detectedLanguage"`
	} `json:"language"`
}

// wireMatch is one LanguageTool match before the offset mapping.
type wireMatch struct {
	Message string
	Offset  int
	Length  int
	RuleID  string
	Values  []string
}

// CheckResult is one complete check: the findings, the language that produced them and the findings
// a term layer suppressed.
type CheckResult struct {
	Findings   []Finding
	Language   string
	Suppressed []Suppression
	Masked     []MaskedFinding
}

// Check sends the text in the given language and returns the unsuppressed findings in body order.
// Large bodies are checked in paragraph chunks; a failure of any chunk leaves the whole check
// unchecked, so a half-checked body is never reported as checked. auto is accepted here for ad-hoc
// checks and reports the detected language; the gate refuses it on write paths.
func (client *Client) Check(ctx context.Context, text, language string) (CheckResult, error) {
	prepared := prepareText(text)
	findings := make([]Finding, 0)
	suppressed := make([]Suppression, 0)
	masked := make([]MaskedFinding, 0)
	addMasked := func(rule, reason string) {
		for index := range masked {
			if masked[index].Rule == rule && masked[index].Reason == reason {
				masked[index].Count++
				return
			}
		}
		masked = append(masked, MaskedFinding{Rule: rule, Count: 1, Reason: reason})
	}
	reported := ""
	for _, span := range chunkPrepared(prepared.text, chunkBytes) {
		matches, detected, err := client.fetch(ctx, prepared.text[span.start:span.end], language)
		if err != nil {
			return CheckResult{}, err
		}
		if reported == "" {
			reported = detected
		}
		for _, match := range matches {
			match.Offset += span.start
			finding, ok := client.finding(text, prepared, match)
			if !ok {
				continue
			}
			if reason := artefactReason(finding, text); reason != "" {
				addMasked(finding.Rule, reason)
				continue
			}
			if entries := client.Terms.Suppress(finding.Offending, finding.Replacements); len(entries) > 0 {
				for index := range entries {
					entries[index].Offset = finding.Offset
					entries[index].Length = finding.Length
				}
				suppressed = append(suppressed, entries...)
				continue
			}
			findings = append(findings, finding)
		}
	}
	findings = mergeFindings(findings)
	for index := range findings {
		findings[index].Index = index + 1
	}
	effective := language
	if language == "auto" {
		if reported != "" {
			effective = reported
		}
	} else if effective == "" {
		effective = reported
	}
	return CheckResult{Findings: findings, Language: effective, Suppressed: suppressed,
		Masked: masked}, nil
}

// Resolve returns the effective check language: the book setting, then the project default, then
// the built-in default.
func Resolve(settings model.Settings, fallback string) string {
	if language := strings.TrimSpace(settings.Language); language != "" {
		return language
	}
	if language := strings.TrimSpace(fallback); language != "" {
		return language
	}
	return DefaultLanguage
}

// finding maps one match from prepared space back to the body.
func (client *Client) finding(text string, prepared preparedText, match wireMatch) (Finding, bool) {
	start := prepared.original(match.Offset)
	end := prepared.original(match.Offset + match.Length)
	if start < 0 || end <= start || prepared.maskedOverlap(start, end) {
		return Finding{}, false
	}
	offending := segment(text, start, end-start)
	line, column := position(text, start)
	return Finding{
		Line: line, Column: column, Offset: start, Length: end - start,
		Rule: match.RuleID, Message: match.Message, Offending: offending,
		Replacements: match.Values,
	}, true
}

// fetch runs one request under the watchdog. The request stays alive across alive intervals; only a
// dead probe starts a new attempt, and only the cap cancels a request against a live server.
func (client *Client) fetch(ctx context.Context, text, language string) ([]wireMatch, string, error) {
	requestCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	type outcome struct {
		matches  []wireMatch
		detected string
		err      error
	}
	result := make(chan outcome, 1)
	go func() {
		matches, detected, err := client.post(requestCtx, text, language)
		result <- outcome{matches: matches, detected: detected, err: err}
	}()
	base := baseURL(client.Endpoint)
	interval := client.Interval()
	timer := time.NewTimer(interval)
	defer timer.Stop()
	alive, failed := 0, 0
	for {
		select {
		case <-ctx.Done():
			return nil, "", ErrTimeout
		case received := <-result:
			return received.matches, received.detected, received.err
		case <-timer.C:
			select {
			case received := <-result:
				return received.matches, received.detected, received.err
			default:
			}
			if reachable(base) {
				alive++
				failed = 0
				if alive >= WatchdogIntervals {
					cancel()
					return nil, "", ErrTimeout
				}
			} else {
				failed++
				alive = 0
				if failed >= 2 {
					cancel()
					return nil, "", ErrUnreachable
				}
			}
			timer.Reset(interval)
		}
	}
}

// post sends one form request and classifies the failure.
func (client *Client) post(ctx context.Context, text, language string) ([]wireMatch, string, error) {
	form := url.Values{}
	form.Set("text", text)
	if strings.TrimSpace(language) == "" {
		language = DefaultLanguage
	}
	form.Set("language", language)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.Endpoint,
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.httpClient().Do(request)
	if err != nil {
		return nil, "", classifyTransport(ctx, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, "", &ErrResponse{Status: response.StatusCode}
	}
	var payload matchesPayload
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&payload); err != nil {
		return nil, "", &ErrResponse{Status: response.StatusCode, Detail: err.Error()}
	}
	matches := make([]wireMatch, 0, len(payload.Matches))
	for _, match := range payload.Matches {
		values := make([]string, 0, len(match.Replacements))
		for _, replacement := range match.Replacements {
			values = append(values, replacement.Value)
		}
		matches = append(matches, wireMatch{
			Message: match.Message, Offset: match.Offset, Length: match.Length,
			RuleID: match.Rule.ID, Values: values,
		})
	}
	detected := strings.TrimSpace(payload.Language.Code)
	if detected == "" || detected == "auto" {
		detected = strings.TrimSpace(payload.Language.DetectedLanguage.Code)
	}
	return matches, detected, nil
}

// classifyTransport separates a canceled watchdog from a dial failure. A dial timeout is an
// unreachable server, not a slow check.
func classifyTransport(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ErrTimeout
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		var opErr *net.OpError
		if errors.As(err, &opErr) && opErr.Op == "dial" {
			return fmt.Errorf("%w: %v", ErrUnreachable, err)
		}
		return ErrTimeout
	}
	return fmt.Errorf("%w: %v", ErrUnreachable, err)
}

// baseURL derives the liveness endpoint of a check URL.
func baseURL(endpoint string) string {
	if base, _, err := parseEndpoint(endpoint); err == nil {
		return base
	}
	if trimmed, ok := strings.CutSuffix(endpoint, "/v2/check"); ok {
		return trimmed
	}
	return strings.TrimRight(endpoint, "/")
}

func (client *Client) httpClient() *http.Client {
	if client.HTTP != nil {
		return client.HTTP
	}
	return http.DefaultClient
}

// Gate applies the policy to a check.
// TermSource names the two paths a check derives its term layers from.
type TermSource struct {
	GlobalPath string
	Dir        string
}

// Load returns the effective set for a language.
func (source TermSource) Load(language string) TermSet {
	return LoadTermSet(source.GlobalPath, source.Dir, language)
}

type Gate struct {
	Client *Client
	Policy Policy
	// Terms is the layered term source for every check.
	Terms TermSource
	// Start starts the shared server on demand. A nil Start uses Serve with the discovered jar.
	Start func(endpoint string) error
	// AdHoc allows language=auto for file and raw checks. Book writes leave it false, so the gate
	// refuses auto before any request.
	AdHoc bool
}

// NewGate returns a gate for the configured endpoint, policy and term layers.
func NewGate(endpoint string, policy Policy, terms TermSource) *Gate {
	return NewGateTimeout(endpoint, policy, terms, 0)
}

// NewGateTimeout returns a gate with an explicit watchdog interval.
func NewGateTimeout(endpoint string, policy Policy, terms TermSource, timeout time.Duration) *Gate {
	return &Gate{Client: NewClientTimeout(endpoint, timeout), Policy: policy, Terms: terms}
}

// Evaluate runs the check in the given language. A request that outlived the watchdog cap is a skip
// with a truthful reason; only an unreachable server triggers the shared start attempt, and a server
// that cannot be started degrades to a reported skip. There is no silent skip: every prose write
// passes this gate. An unsupported language is skipped with grammar.language-unsupported and never
// replaced by a fallback.
func (gate *Gate) Evaluate(ctx context.Context, block model.ID, text, language string) Report {
	report := Report{Policy: string(PolicyBlock)}
	if gate == nil || gate.Client == nil {
		return report
	}
	report.Policy = string(gate.Policy)
	language = strings.TrimSpace(language)
	if language == "" {
		language = DefaultLanguage
	}
	if language == "auto" && !gate.AdHoc {
		return gate.skippedReason(ReasonLanguageUnsupported, "auto is ad-hoc only")
	}
	report.Language = language
	if language != "auto" {
		canonical, ok, err := gate.Client.Supported(ctx, language)
		if err != nil {
			if ReasonFor(err) != ReasonUnreachable {
				return gate.skippedReport(report, ReasonFor(err), err)
			}
			if startErr := gate.start(); startErr != nil {
				return gate.skippedReport(report, ReasonStartFailed, startErr)
			}
			canonical, ok, err = gate.Client.Supported(ctx, language)
			if err != nil {
				return gate.skippedReport(report, ReasonFor(err), err)
			}
		}
		if !ok {
			return gate.skippedReason(ReasonLanguageUnsupported,
				fmt.Sprintf("the server does not report the language %q", language))
		}
		language = canonical.Code
		report.Language = language
	}
	gate.Client.Terms = gate.Terms.Load(language)
	findings, err := gate.Client.Check(ctx, text, language)
	if err != nil {
		if ReasonFor(err) != ReasonUnreachable {
			return gate.skippedReport(report, ReasonFor(err), err)
		}
		if startErr := gate.start(); startErr != nil {
			return gate.skippedReport(report, ReasonStartFailed, startErr)
		}
		findings, err = gate.Client.Check(ctx, text, language)
		if err != nil {
			return gate.skippedReport(report, ReasonFor(err), err)
		}
	}
	for index := range findings.Findings {
		findings.Findings[index].Block = block
	}
	report.Checked = true
	report.Language = findings.Language
	report.Findings = findings.Findings
	report.Suppressed = findings.Suppressed
	report.Masked = findings.Masked
	return report
}

// skippedReport keeps the language of an already-started evaluation.
func (gate *Gate) skippedReport(report Report, reason string, detail error) Report {
	report.Checked = false
	report.Reason = reason
	report.Degraded = fmt.Sprintf("grammar check skipped: %v", detail)
	report.Findings = nil
	return report
}

// skippedReason reports a skip that never reached a request.
func (gate *Gate) skippedReason(reason, message string) Report {
	return Report{
		Policy:   string(gate.Policy),
		Reason:   reason,
		Degraded: "grammar check skipped: " + message,
	}
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

// maskedSpan is one original range the prose check must ignore: a code span or a Markdown marker.
type maskedSpan struct {
	start int
	end   int
}

// preparedText is the prose view sent to the language check plus the map back to the original body.
// Fenced blocks are dropped; inline code becomes the word "code"; Markdown markers become spaces; the
// map keeps every finding on real prose with its original offsets.
type preparedText struct {
	text    string
	offsets []int
	spans   []maskedSpan
}

// prepareText removes code and Markdown structure from the check without losing the offsets. The
// language check is about prose; a command example, an identifier or a bold marker must not produce a
// prose finding. The mask is language independent and byte preserving, so every offset stays valid.
func prepareText(text string) preparedText {
	var builder strings.Builder
	offsets := make([]int, 0, len(text))
	spans := make([]maskedSpan, 0)
	original := 0
	inFence := false
	for _, line := range strings.SplitAfter(text, "\n") {
		lineStart := original
		original += len(line)
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			builder.WriteByte('\n')
			offsets = append(offsets, lineStart+len(line)-1)
			continue
		}
		if inFence {
			builder.WriteByte('\n')
			offsets = append(offsets, lineStart+len(line)-1)
			continue
		}
		markers := markupMask(line)
		for index := 0; index < len(line); {
			if line[index] == '`' {
				end := index + 1
				for end < len(line) && line[end] != '`' {
					end++
				}
				if end < len(line) {
					spans = append(spans, maskedSpan{start: lineStart + index, end: lineStart + end + 1})
					builder.WriteString("code")
					for count := 0; count < len("code"); count++ {
						offsets = append(offsets, lineStart+index)
					}
					index = end + 1
					continue
				}
			}
			if markers[index] {
				builder.WriteByte(' ')
			} else {
				builder.WriteByte(line[index])
			}
			offsets = append(offsets, lineStart+index)
			index++
		}
		// Coalesce the masked bytes of this line into spans, so a finding that touches a marker is
		// dropped exactly like a code finding.
		runStart := -1
		for index := 0; index <= len(line); index++ {
			if index < len(line) && markers[index] {
				if runStart < 0 {
					runStart = index
				}
				continue
			}
			if runStart >= 0 {
				spans = append(spans, maskedSpan{start: lineStart + runStart, end: lineStart + index})
				runStart = -1
			}
		}
	}
	offsets = append(offsets, len(text))
	return preparedText{text: builder.String(), offsets: offsets, spans: spans}
}

// markupMask marks the Markdown structure bytes of one line: blockquote and list markers, table
// pipes, emphasis delimiters, inline HTML tags, links and reference definitions. Only markers are
// marked, never the prose they wrap; an intra-word underscore stays content.
func markupMask(line string) []bool {
	markers := make([]bool, len(line))
	mark := func(start, end int) {
		for index := start; index < end && index < len(markers); index++ {
			markers[index] = true
		}
	}
	start := 0
	for start < len(line) && (line[start] == ' ' || line[start] == '\t') {
		start++
	}
	if start < len(line) && line[start] == '>' {
		end := start + 1
		if end < len(line) && line[end] == ' ' {
			end++
		}
		mark(start, end)
		start = end
	}
	switch {
	case start+1 < len(line) && (line[start] == '-' || line[start] == '+' || line[start] == '*') &&
		line[start+1] == ' ':
		mark(start, start+2)
		start += 2
	default:
		digits := start
		for digits < len(line) && line[digits] >= '0' && line[digits] <= '9' {
			digits++
		}
		if digits > start && digits+1 < len(line) &&
			(line[digits] == '.' || line[digits] == ')') && line[digits+1] == ' ' {
			mark(start, digits+2)
			start = digits + 2
		}
	}
	if isReferenceDefinition(line, start) {
		mark(start, len(line))
		return markers
	}
	for index := start; index < len(line); index++ {
		switch line[index] {
		case '|', '*':
			mark(index, index+1)
		case '_':
			if emphasisDelimiter(line, index) {
				mark(index, index+1)
			}
		case '~':
			if index+1 < len(line) && line[index+1] == '~' {
				mark(index, index+2)
				index++
			}
		case '<':
			if end := htmlTagEnd(line, index); end > index {
				mark(index, end)
				index = end - 1
			}
		case '[':
			if end := maskLink(line, index, mark); end > index {
				index = end - 1
			}
		case '!':
			if index+1 < len(line) && line[index+1] == '[' {
				mark(index, index+1)
				if end := maskLink(line, index+1, mark); end > index {
					index = end - 1
				}
			}
		}
	}
	return markers
}

// emphasisDelimiter reports whether an underscore or star is a delimiter rather than an intra-word
// character, so `snake_case` stays one word and `_word_` loses its markers.
func emphasisDelimiter(line string, index int) bool {
	before := index > 0 && isWordByte(line[index-1])
	after := index+1 < len(line) && isWordByte(line[index+1])
	return !(before && after)
}

func isWordByte(value byte) bool {
	return value >= '0' && value <= '9' || value >= 'a' && value <= 'z' ||
		value >= 'A' && value <= 'Z'
}

// htmlTagEnd returns the end offset of an inline HTML tag at index, or zero when the angle bracket
// is ordinary text such as "<10".
func htmlTagEnd(line string, index int) int {
	if index+1 >= len(line) {
		return 0
	}
	next := line[index+1]
	if next != '/' && next != '!' && !isWordByte(next) {
		return 0
	}
	offset := strings.IndexByte(line[index:], '>')
	if offset < 0 {
		return 0
	}
	return index + offset + 1
}

// isReferenceDefinition reports a link definition line, which carries a target and no prose.
func isReferenceDefinition(line string, start int) bool {
	if start >= len(line) || line[start] != '[' {
		return false
	}
	offset := strings.IndexByte(line[start:], ']')
	if offset < 0 {
		return false
	}
	rest := line[start+offset+1:]
	return strings.HasPrefix(rest, ":") && strings.TrimSpace(rest[1:]) != ""
}

// maskLink marks a link or image at the bracket, keeping the visible text. It returns the offset
// after the link.
func maskLink(line string, index int, mark func(int, int)) int {
	offset := strings.IndexByte(line[index:], ']')
	if offset < 0 {
		return index
	}
	close := index + offset
	mark(index, index+1)
	mark(close, close+1)
	end := close + 1
	switch {
	case end < len(line) && line[end] == '(':
		if destination := strings.IndexByte(line[end:], ')'); destination >= 0 {
			mark(end, end+destination+1)
			end += destination + 1
		}
	case end < len(line) && line[end] == '[':
		if reference := strings.IndexByte(line[end:], ']'); reference >= 0 {
			mark(end, end+reference+1)
			end += reference + 1
		}
	}
	return end
}

// artefactReason names the documented false positive a finding is, or returns "". The table is keyed
// by the rule the server emitted, so a new artefact is one clause and never a language branch.
func artefactReason(finding Finding, text string) string {
	if finding.Rule == "DE_CASE" && touchesSpacedDash(text, finding.Offset, finding.Offset+finding.Length) {
		return "the finding touches a spaced dash"
	}
	return ""
}

// touchesSpacedDash reports whether a finding range or its immediate neighbourhood contains an em or
// en dash with a space on both sides. Some servers anchor the finding on the dash itself and select
// the following word as the replacement, so the range alone is not enough.
func touchesSpacedDash(text string, start, end int) bool {
	if start < 0 {
		start = 0
	}
	if end > len(text) {
		end = len(text)
	}
	const window = 6
	if from := start - window; from > 0 {
		start = from
	} else {
		start = 0
	}
	if to := end + window; to < len(text) {
		end = to
	} else {
		end = len(text)
	}
	region := text[start:end]
	return strings.Contains(region, " — ") || strings.Contains(region, " – ")
}

// original maps one offset of the prepared text back to the body.
func (prepared preparedText) original(index int) int {
	if index < 0 {
		return -1
	}
	if index >= len(prepared.offsets) {
		return prepared.offsets[len(prepared.offsets)-1]
	}
	return prepared.offsets[index]
}

// maskedOverlap reports whether an original range intersects a masked span.
func (prepared preparedText) maskedOverlap(start, end int) bool {
	for _, span := range prepared.spans {
		if start < span.end && span.start < end {
			return true
		}
	}
	return false
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
