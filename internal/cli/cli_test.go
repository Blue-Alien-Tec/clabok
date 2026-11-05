package cli_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
)

type testError struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	Hint    string          `json:"hint"`
	Details json.RawMessage `json:"details"`
}

type testEnvelope struct {
	Schema  int             `json:"schema"`
	OK      bool            `json:"ok"`
	Command string          `json:"command"`
	Data    json.RawMessage `json:"data"`
	Error   *testError      `json:"error"`
}

func run(t *testing.T, registry *cli.Registry, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr strings.Builder
	code := cli.Run(registry, args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func envelopeOf(t *testing.T, text string) testEnvelope {
	t.Helper()
	var payload testEnvelope
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("not a JSON envelope: %v\n%s", err, text)
	}
	return payload
}

type dryRunReceipt struct {
	DryRun bool `json:"dryRun"`
}

func (receipt dryRunReceipt) DryRunResult() bool { return receipt.DryRun }

func (receipt dryRunReceipt) Text() string { return "dry run: nothing written" }

func TestVersionJSON(t *testing.T) {
	code, stdout, stderr := run(t, cli.NewRegistry(), "version")
	if code != cli.ExitOK {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr must stay empty, got %q", stderr)
	}
	payload := envelopeOf(t, stdout)
	if payload.Schema != cli.SchemaVersion {
		t.Fatalf("schema %d", payload.Schema)
	}
	if !payload.OK || payload.Command != "version" {
		t.Fatalf("unexpected envelope %+v", payload)
	}
	var data map[string]string
	if err := json.Unmarshal(payload.Data, &data); err != nil {
		t.Fatal(err)
	}
	if data["product"] != "CLABOK" || data["cli"] != "pecia" {
		t.Fatalf("unexpected data %v", data)
	}
}

func TestVersionText(t *testing.T) {
	code, stdout, _ := run(t, cli.NewRegistry(), "version", "--text")
	if code != cli.ExitOK {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stdout, "CLABOK") || strings.Contains(stdout, "{") {
		t.Fatalf("unexpected text output %q", stdout)
	}
}

func TestMissingCommand(t *testing.T) {
	code, stdout, stderr := run(t, cli.NewRegistry())
	if code != cli.ExitUsage {
		t.Fatalf("exit %d", code)
	}
	if stdout != "" {
		t.Fatalf("stdout must stay empty, got %q", stdout)
	}
	payload := envelopeOf(t, stderr)
	if payload.OK || payload.Error == nil || payload.Error.Code != "usage.missing-command" {
		t.Fatalf("unexpected error %+v", payload.Error)
	}
}

func TestUnknownCommandSuggests(t *testing.T) {
	code, _, stderr := run(t, cli.NewRegistry(), "versoin")
	if code != cli.ExitUsage {
		t.Fatalf("exit %d", code)
	}
	payload := envelopeOf(t, stderr)
	if payload.Error == nil || payload.Error.Code != "usage.unknown-command" {
		t.Fatalf("unexpected error %+v", payload.Error)
	}
	if !strings.Contains(string(payload.Error.Details), `"version"`) {
		t.Fatalf("missing suggestion: %s", payload.Error.Details)
	}
}

func TestMissingSubcommand(t *testing.T) {
	registry := cli.NewRegistry()
	registry.Add(&cli.Command{
		Name:    "group sub",
		Summary: "a subcommand",
		Run: func(*cli.Env, []string) (*cli.Result, error) {
			return &cli.Result{Data: map[string]any{"ok": true}}, nil
		},
	})
	code, _, stderr := run(t, registry, "group")
	if code != cli.ExitUsage {
		t.Fatalf("exit %d", code)
	}
	payload := envelopeOf(t, stderr)
	if payload.Error.Code != "usage.missing-subcommand" {
		t.Fatalf("unexpected code %q", payload.Error.Code)
	}
}

func TestUnknownFlag(t *testing.T) {
	code, stdout, stderr := run(t, cli.NewRegistry(), "version", "--bogus")
	if code != cli.ExitUsage {
		t.Fatalf("exit %d", code)
	}
	if stdout != "" {
		t.Fatalf("stdout must stay empty, got %q", stdout)
	}
	payload := envelopeOf(t, stderr)
	if payload.Error == nil || payload.Error.Code != "usage.unknown-flag" {
		t.Fatalf("unexpected error %+v", payload.Error)
	}
}

func echoCommand() *cli.Command {
	return &cli.Command{
		Name:    "echo",
		Summary: "echo the resolved globals",
		Run: func(env *cli.Env, args []string) (*cli.Result, error) {
			return &cli.Result{Data: map[string]any{
				"project": env.Globals.Project,
				"config":  env.Globals.Config,
				"dryRun":  env.Globals.DryRun,
				"args":    args,
			}}, nil
		},
	}
}

func TestGlobalsBeforeAndAfterCommand(t *testing.T) {
	registry := cli.NewRegistry()
	registry.Add(echoCommand())

	code, stdout, _ := run(t, registry, "--project", "/before", "echo")
	if code != cli.ExitOK {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stdout, "/before") {
		t.Fatalf("global before command lost: %s", stdout)
	}

	code, stdout, _ = run(t, registry, "--project=/after", "echo")
	if code != cli.ExitOK || !strings.Contains(stdout, "/after") {
		t.Fatalf("equals form lost: %d %s", code, stdout)
	}

	code, stdout, _ = run(t, registry, "echo", "--project", "/late")
	if code != cli.ExitOK || !strings.Contains(stdout, "/late") {
		t.Fatalf("global after command lost: %d %s", code, stdout)
	}
}

func TestTerminatorPassesPositionals(t *testing.T) {
	registry := cli.NewRegistry()
	registry.Add(echoCommand())
	code, stdout, _ := run(t, registry, "echo", "--", "--project")
	if code != cli.ExitOK {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stdout, `"--project"`) {
		t.Fatalf("terminator not honoured: %s", stdout)
	}
}

func TestDryRunContract(t *testing.T) {
	plain := &cli.Command{
		Name:     "plain",
		Mutating: true,
		Run: func(*cli.Env, []string) (*cli.Result, error) {
			return &cli.Result{Data: map[string]any{"changed": true}}, nil
		},
	}
	receipt := &cli.Command{
		Name:     "receipt",
		Mutating: true,
		Run: func(*cli.Env, []string) (*cli.Result, error) {
			return &cli.Result{Data: dryRunReceipt{DryRun: true}}, nil
		},
	}
	registry := cli.NewRegistry()
	registry.Add(plain)
	registry.Add(receipt)

	code, _, stderr := run(t, registry, "plain", "--dry-run")
	if code != cli.ExitFailure {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stderr, "internal.dry-run-missing") {
		t.Fatalf("unexpected stderr %s", stderr)
	}

	if code, _, _ := run(t, registry, "receipt", "--dry-run"); code != cli.ExitOK {
		t.Fatalf("receipt exit %d", code)
	}
	if code, _, _ := run(t, registry, "plain"); code != cli.ExitOK {
		t.Fatalf("plain without dry-run exit %d", code)
	}
}

func TestTextFallback(t *testing.T) {
	registry := cli.NewRegistry()
	registry.Add(echoCommand())
	code, stdout, _ := run(t, registry, "echo", "--text")
	if code != cli.ExitOK {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stdout, `"project"`) {
		t.Fatalf("fallback text missing: %q", stdout)
	}
}

func TestPanicIsRecovered(t *testing.T) {
	registry := cli.NewRegistry()
	registry.Add(&cli.Command{
		Name: "boom",
		Run: func(*cli.Env, []string) (*cli.Result, error) {
			panic("kaboom")
		},
	})
	code, stdout, stderr := run(t, registry, "boom")
	if code != cli.ExitFailure {
		t.Fatalf("exit %d", code)
	}
	if stdout != "" {
		t.Fatalf("stdout must stay empty, got %q", stdout)
	}
	if !strings.Contains(stderr, "panic: kaboom") {
		t.Fatalf("missing panic diagnostic: %s", stderr)
	}
	if !strings.Contains(stderr, "internal.panic") {
		t.Fatalf("missing panic envelope: %s", stderr)
	}
}

func TestHelp(t *testing.T) {
	code, stdout, _ := run(t, cli.NewRegistry(), "help")
	if code != cli.ExitOK {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stdout, "version") || !strings.Contains(stdout, "help") {
		t.Fatalf("help does not list the builtins: %s", stdout)
	}

	code, stdout, _ = run(t, cli.NewRegistry(), "help", "--text")
	if code != cli.ExitOK || !strings.Contains(stdout, "usage:") {
		t.Fatalf("unexpected help text %q", stdout)
	}

	code, stdout, _ = run(t, cli.NewRegistry(), "help", "version")
	if code != cli.ExitOK {
		t.Fatalf("exit %d", code)
	}
	payload := envelopeOf(t, stdout)
	var info struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(payload.Data, &info); err != nil || info.Name != "version" {
		t.Fatalf("unexpected help payload %s", payload.Data)
	}
}

func TestHelpAfterCommandFlag(t *testing.T) {
	registry := cli.NewRegistry()
	registry.Add(echoCommand())
	code, stdout, _ := run(t, registry, "echo", "--help")
	if code != cli.ExitOK {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stdout, "echo") {
		t.Fatalf("missing command help: %s", stdout)
	}
}
