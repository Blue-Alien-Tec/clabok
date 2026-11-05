package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
)

type rootValues struct {
	Root string `json:"root"`
}

type rollbackValues struct {
	UndoLimit      int `json:"undoLimit"`
	BookPointLimit int `json:"bookPointLimit"`
}

type grammarValues struct {
	Endpoint string `json:"endpoint"`
}

type daemonValues struct {
	Socket string `json:"socket"`
}

type concurrencyValues struct {
	Mode   string       `json:"mode"`
	Daemon daemonValues `json:"daemon"`
}

type asciiuiValues struct {
	Types map[string]string `json:"types"`
}

type showValues struct {
	Store       rootValues        `json:"store"`
	Temp        rootValues        `json:"temp"`
	Docs        rootValues        `json:"docs"`
	Rollback    rollbackValues    `json:"rollback"`
	Grammar     grammarValues     `json:"grammar"`
	Concurrency concurrencyValues `json:"concurrency"`
	Asciiui     asciiuiValues     `json:"asciiui"`
}

type showPayload struct {
	File       string     `json:"file"`
	ProjectDir string     `json:"projectDir"`
	Exists     bool       `json:"exists"`
	Values     showValues `json:"values"`
}

func (payload showPayload) Text() string {
	file := payload.File
	if file == "" {
		file = "(none; built-in defaults)"
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "file: %s\n", file)
	fmt.Fprintf(&builder, "project: %s\n", payload.ProjectDir)
	fmt.Fprintf(&builder, "store: %s\n", payload.Values.Store.Root)
	fmt.Fprintf(&builder, "temp: %s\n", payload.Values.Temp.Root)
	fmt.Fprintf(&builder, "docs: %s\n", payload.Values.Docs.Root)
	fmt.Fprintf(&builder, "rollback: undo=%d book=%d\n",
		payload.Values.Rollback.UndoLimit, payload.Values.Rollback.BookPointLimit)
	fmt.Fprintf(&builder, "concurrency: %s\n", payload.Values.Concurrency.Mode)
	fmt.Fprintf(&builder, "grammar: %s\n", payload.Values.Grammar.Endpoint)
	fmt.Fprintf(&builder, "asciiui: %d override(s)\n", len(payload.Values.Asciiui.Types))
	return builder.String()
}

type getPayload struct {
	Key    string `json:"key"`
	Value  any    `json:"value"`
	Source string `json:"source"`
}

func (payload getPayload) Text() string {
	return fmt.Sprintf("%s: %v (%s)", payload.Key, payload.Value, payload.Source)
}

type changePayload struct {
	DryRun  bool   `json:"dryRun"`
	File    string `json:"file"`
	Key     string `json:"key"`
	From    any    `json:"from"`
	To      any    `json:"to"`
	Changed bool   `json:"changed"`
}

// DryRunResult marks the payload as a dry-run receipt.
func (payload changePayload) DryRunResult() bool { return payload.DryRun }

func (payload changePayload) Text() string {
	prefix := ""
	if payload.DryRun {
		prefix = "dry run: "
	}
	if !payload.Changed {
		return prefix + payload.Key + ": unchanged"
	}
	return fmt.Sprintf("%s%s: %v -> %v", prefix, payload.Key, payload.From, payload.To)
}

type pathPayload struct {
	File       string `json:"file"`
	ProjectDir string `json:"projectDir"`
	Exists     bool   `json:"exists"`
}

func (payload pathPayload) Text() string {
	if payload.File == "" {
		return fmt.Sprintf("no configuration file; defaults apply in %s", payload.ProjectDir)
	}
	return fmt.Sprintf("%s (exists: %t)", payload.File, payload.Exists)
}

type validPayload struct {
	Valid      bool   `json:"valid"`
	File       string `json:"file"`
	ProjectDir string `json:"projectDir"`
}

func (payload validPayload) Text() string {
	if payload.File == "" {
		return "valid: no configuration file; built-in defaults apply"
	}
	return "valid: " + payload.File
}

// Register adds the config command group to the registry.
func Register(registry *cli.Registry) {
	registry.Add(&cli.Command{
		Name:    "config show",
		Summary: "show the effective configuration",
		Usage:   "pecia config show",
		Run:     runShow,
	})
	registry.Add(&cli.Command{
		Name:    "config get",
		Summary: "print one effective value",
		Usage:   "pecia config get <key>",
		Run:     runGet,
	})
	registry.Add(&cli.Command{
		Name:     "config set",
		Summary:  "set one value",
		Usage:    "pecia config set <key> <value>",
		Mutating: true,
		Run:      runSet,
	})
	registry.Add(&cli.Command{
		Name:     "config unset",
		Summary:  "restore one value to its default",
		Usage:    "pecia config unset <key>",
		Mutating: true,
		Run:      runUnset,
	})
	registry.Add(&cli.Command{
		Name:    "config validate",
		Summary: "check the configuration",
		Usage:   "pecia config validate",
		Run:     runValidate,
	})
	registry.Add(&cli.Command{
		Name:    "config path",
		Summary: "print the resolved configuration file",
		Usage:   "pecia config path",
		Run:     runPath,
	})
}

func runShow(env *cli.Env, args []string) (*cli.Result, error) {
	if len(args) > 0 {
		return nil, cli.Usage("usage.unexpected-argument", "config show takes no arguments")
	}
	values, err := Load(env)
	if err != nil {
		return nil, err
	}
	return &cli.Result{Data: showPayload{
		File:       values.File,
		ProjectDir: values.ProjectDir,
		Exists:     values.Exists,
		Values: showValues{
			Store:    rootValues{Root: values.StoreRoot},
			Temp:     rootValues{Root: values.TempRoot},
			Docs:     rootValues{Root: values.DocsRoot},
			Rollback: rollbackValues{UndoLimit: values.UndoLimit, BookPointLimit: values.BookPointLimit},
			Grammar:  grammarValues{Endpoint: values.GrammarEndpoint},
			Concurrency: concurrencyValues{
				Mode:   values.ConcurrencyMode,
				Daemon: daemonValues{Socket: values.DaemonSocket},
			},
			Asciiui: asciiuiValues{Types: values.AsciiUITypes},
		},
	}}, nil
}

func runGet(env *cli.Env, args []string) (*cli.Result, error) {
	if len(args) != 1 {
		return nil, cli.Usage("usage.invalid-arguments", "config get takes exactly one key").
			WithHint("run `pecia config show` for the effective values")
	}
	document, location, err := readDocument(env, false)
	if err != nil {
		return nil, err
	}
	value, source, valueErr := getValue(document, location, env.Globals, args[0])
	if valueErr != nil {
		return nil, valueErr
	}
	return &cli.Result{Data: getPayload{Key: args[0], Value: value, Source: source}}, nil
}

func runSet(env *cli.Env, args []string) (*cli.Result, error) {
	if len(args) != 2 {
		return nil, cli.Usage("usage.invalid-arguments", "config set takes a key and one value").
			WithHint("pecia config set <key> <value>")
	}
	key, raw := args[0], args[1]
	document, location, err := readDocument(env, true)
	if err != nil {
		return nil, err
	}
	from, _, fromErr := lookupValue(document, key)
	if fromErr != nil {
		return nil, fromErr
	}
	if setErr := setValue(document, key, raw); setErr != nil {
		return nil, setErr
	}
	if issues := validate(document); len(issues) > 0 {
		return nil, invalidError(issues)
	}
	to, _, toErr := lookupValue(document, key)
	if toErr != nil {
		return nil, toErr
	}
	changed := !sameValue(from, to) || !location.Exists
	target := targetFile(location)
	if env.Globals.DryRun {
		return &cli.Result{Data: changePayload{
			DryRun: true, File: target, Key: key, From: from, To: to, Changed: changed,
		}}, nil
	}
	if changed {
		if writeErr := writeFile(target, document); writeErr != nil {
			return nil, writeErr
		}
	}
	return &cli.Result{Data: changePayload{
		File: target, Key: key, From: from, To: to, Changed: changed,
	}}, nil
}

func runUnset(env *cli.Env, args []string) (*cli.Result, error) {
	if len(args) != 1 {
		return nil, cli.Usage("usage.invalid-arguments", "config unset takes exactly one key")
	}
	key := args[0]
	document, location, err := readDocument(env, true)
	if err != nil {
		return nil, err
	}
	from, source, fromErr := lookupValue(document, key)
	if fromErr != nil {
		return nil, fromErr
	}
	if unsetErr := unsetValue(document, key); unsetErr != nil {
		return nil, unsetErr
	}
	to, _, toErr := lookupValue(document, key)
	if toErr != nil {
		return nil, toErr
	}
	changed := source == "file"
	target := targetFile(location)
	if env.Globals.DryRun {
		return &cli.Result{Data: changePayload{
			DryRun: true, File: target, Key: key, From: from, To: to, Changed: changed,
		}}, nil
	}
	if changed {
		if writeErr := writeFile(target, document); writeErr != nil {
			return nil, writeErr
		}
	}
	return &cli.Result{Data: changePayload{
		File: target, Key: key, From: from, To: to, Changed: changed,
	}}, nil
}

func runValidate(env *cli.Env, args []string) (*cli.Result, error) {
	if len(args) > 0 {
		return nil, cli.Usage("usage.unexpected-argument", "config validate takes no arguments")
	}
	_, location, err := readDocument(env, false)
	if err != nil {
		return nil, err
	}
	return &cli.Result{Data: validPayload{
		Valid:      true,
		File:       location.File,
		ProjectDir: location.ProjectDir,
	}}, nil
}

func runPath(env *cli.Env, args []string) (*cli.Result, error) {
	if len(args) > 0 {
		return nil, cli.Usage("usage.unexpected-argument", "config path takes no arguments")
	}
	location, err := discover(env.Globals)
	if err != nil {
		return nil, err
	}
	return &cli.Result{Data: pathPayload{
		File:       location.File,
		ProjectDir: location.ProjectDir,
		Exists:     location.Exists,
	}}, nil
}

func invalidError(issues []Issue) *cli.Error {
	return cli.Fail("config.invalid", "invalid configuration: %d problem(s)", len(issues)).
		WithHint("fix the reported keys and run the command again").
		WithDetails(issuesPayload{Issues: issues})
}

func targetFile(location Location) string {
	if location.File != "" {
		return location.File
	}
	return filepath.Join(location.ProjectDir, FileName)
}

func sameValue(left, right any) bool {
	return reflect.DeepEqual(left, right)
}

func writeFile(path string, document *file) *cli.Error {
	data, err := encode(document)
	if err != nil {
		return cli.Fail("config.write-failed", "encode: %v", err)
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return cli.Fail("config.write-failed", "%v", err)
	}
	temporary, err := os.CreateTemp(directory, ".clabok-*.tmp")
	if err != nil {
		return cli.Fail("config.write-failed", "%v", err)
	}
	name := temporary.Name()
	discard := func() {
		_ = temporary.Close()
		_ = os.Remove(name)
	}
	if _, err := temporary.Write(data); err != nil {
		discard()
		return cli.Fail("config.write-failed", "%v", err)
	}
	if err := temporary.Sync(); err != nil {
		discard()
		return cli.Fail("config.write-failed", "%v", err)
	}
	if err := temporary.Close(); err != nil {
		_ = os.Remove(name)
		return cli.Fail("config.write-failed", "%v", err)
	}
	if err := os.Chmod(name, 0o644); err != nil {
		_ = os.Remove(name)
		return cli.Fail("config.write-failed", "%v", err)
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return cli.Fail("config.write-failed", "%v", err)
	}
	return nil
}
