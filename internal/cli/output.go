package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

// SchemaVersion versions the JSON envelope. It changes only on a breaking change.
const SchemaVersion = 1

// Exit codes. A reject is a distinct outcome so tooling can branch without parsing messages.
const (
	ExitOK      = 0
	ExitFailure = 1
	ExitUsage   = 2
	ExitReject  = 3
)

// Texter is implemented by payloads that offer a human-readable form.
type Texter interface {
	Text() string
}

// DryRunResult marks a payload as a dry-run receipt.
type DryRunResult interface {
	DryRunResult() bool
}

// Result is what a command returns.
type Result struct {
	Data any
	Text string
	// Raw, when set, is written verbatim instead of the JSON envelope. It carries an output format
	// the envelope cannot express, such as NDJSON.
	Raw []byte
}

type errorObject struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
	Details any    `json:"details,omitempty"`
}

type envelope struct {
	Schema  int          `json:"schema"`
	OK      bool         `json:"ok"`
	Command string       `json:"command"`
	Data    any          `json:"data,omitempty"`
	Error   *errorObject `json:"error,omitempty"`
}

// ExitCode maps an outcome kind to the process exit code.
func ExitCode(kind Kind) int {
	switch kind {
	case KindUsage:
		return ExitUsage
	case KindReject:
		return ExitReject
	default:
		return ExitFailure
	}
}

func writeJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func textOf(result *Result) string {
	if result == nil {
		return ""
	}
	if result.Text != "" {
		return result.Text
	}
	if texter, ok := result.Data.(Texter); ok {
		return texter.Text()
	}
	// A successful command must not fail because its text form is missing.
	var buffer bytes.Buffer
	if err := writeJSON(&buffer, result.Data); err != nil {
		return ""
	}
	return buffer.String()
}

func ensureTrailingNewline(text string) string {
	if text == "" || strings.HasSuffix(text, "\n") {
		return text
	}
	return text + "\n"
}
