package config

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
)

// Location is where the configuration was found (or would be created).
type Location struct {
	File       string
	ProjectDir string
	WorkingDir string
	Exists     bool
	Explicit   bool
}

// Values are the effective configuration values.
type Values struct {
	File            string
	ProjectDir      string
	Exists          bool
	StoreRoot       string
	TempRoot        string
	DocsRoot        string
	UndoLimit       int
	BookPointLimit  int
	GrammarEndpoint string
	GrammarPolicy   string
	GrammarTimeout  time.Duration
	GrammarLanguage string
	ConcurrencyMode string
	DaemonSocket    string
	AsciiUITypes    map[string]string
}

// Load returns the effective configuration for a command.
func Load(env *cli.Env) (*Values, *cli.Error) {
	document, location, err := readDocument(env, false)
	if err != nil {
		return nil, err
	}
	return resolve(document, location, env.Globals), nil
}

func readDocument(env *cli.Env, allowMissingExplicit bool) (*file, Location, *cli.Error) {
	location, err := discover(env.Globals)
	if err != nil {
		return nil, Location{}, err
	}
	if !location.Exists {
		if location.Explicit && !allowMissingExplicit {
			return nil, location, cli.Fail("config.not-found", "no configuration file at %s", location.File).
				WithHint("run `pecia config set <key> <value>` to create it")
		}
		return &file{}, location, nil
	}
	data, readErr := os.ReadFile(location.File)
	if readErr != nil {
		return nil, location, cli.Fail("config.read-failed", "%v", readErr)
	}
	document, decodeErr := decode(data)
	if decodeErr != nil {
		return nil, location, decodeErr
	}
	if issues := validate(document); len(issues) > 0 {
		return nil, location, cli.Fail("config.invalid", "invalid configuration: %d problem(s)", len(issues)).
			WithHint("fix the reported keys and run the command again").
			WithDetails(issuesPayload{Issues: issues})
	}
	return document, location, nil
}

func discover(globals cli.Globals) (Location, *cli.Error) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return Location{}, cli.Fail("config.cwd-unavailable", "%v", err)
	}

	if globals.Config != "" {
		absolute, absErr := filepath.Abs(globals.Config)
		if absErr != nil {
			return Location{}, cli.Usage("usage.invalid-value", "--config: %v", absErr)
		}
		info, statErr := os.Stat(absolute)
		if statErr == nil && info.IsDir() {
			return Location{}, cli.Usage("usage.invalid-value", "--config points at a directory: %s", absolute)
		}
		return Location{
			File:       absolute,
			ProjectDir: filepath.Dir(absolute),
			WorkingDir: workingDirectory,
			Exists:     statErr == nil,
			Explicit:   true,
		}, nil
	}

	start := workingDirectory
	if globals.Project != "" {
		absolute, absErr := filepath.Abs(globals.Project)
		if absErr != nil {
			return Location{}, cli.Usage("usage.invalid-value", "--project: %v", absErr)
		}
		info, statErr := os.Stat(absolute)
		if statErr != nil || !info.IsDir() {
			return Location{}, cli.Usage("usage.invalid-value", "--project is not a directory: %s", absolute).
				WithHint("pass the project root that contains clabok.json")
		}
		start = absolute
	}

	directory := start
	for {
		candidate := filepath.Join(directory, FileName)
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			return Location{
				File:       candidate,
				ProjectDir: directory,
				WorkingDir: workingDirectory,
				Exists:     true,
			}, nil
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			break
		}
		directory = parent
	}
	return Location{ProjectDir: start, WorkingDir: workingDirectory}, nil
}

func resolve(document *file, location Location, globals cli.Globals) *Values {
	values := &Values{
		File:            location.File,
		ProjectDir:      location.ProjectDir,
		Exists:          location.Exists,
		UndoLimit:       builtin.UndoLimit,
		BookPointLimit:  builtin.BookPointLimit,
		GrammarEndpoint: builtin.GrammarEndpoint,
		GrammarPolicy:   builtin.GrammarPolicy,
		GrammarTimeout:  parseDuration(builtin.GrammarTimeout, 5*time.Second),
		GrammarLanguage: builtin.GrammarLanguage,
		ConcurrencyMode: builtin.ConcurrencyMode,
		DaemonSocket:    builtin.DaemonSocket,
		AsciiUITypes:    map[string]string{},
	}
	values.StoreRoot = resolveRoot(document.storeRoot(), builtin.StoreRoot, location, globals.StoreRoot)
	values.TempRoot = resolveRoot(document.tempRoot(), builtin.TempRoot, location, globals.TempRoot)
	values.DocsRoot = resolveRoot(document.docsRoot(), builtin.DocsRoot, location, globals.DocsRoot)
	if limit, ok := document.undoLimit(); ok {
		values.UndoLimit = limit
	}
	if limit, ok := document.bookPointLimit(); ok {
		values.BookPointLimit = limit
	}
	if endpoint := document.grammarEndpoint(); endpoint != "" {
		values.GrammarEndpoint = endpoint
	}
	if policy := document.grammarPolicy(); policy != "" {
		values.GrammarPolicy = policy
	}
	if timeout := document.grammarTimeout(); timeout != "" {
		values.GrammarTimeout = parseDuration(timeout, values.GrammarTimeout)
	}
	if language := strings.TrimSpace(document.grammarLanguage()); language != "" {
		values.GrammarLanguage = language
	}
	if mode := document.concurrencyMode(); mode != "" {
		values.ConcurrencyMode = mode
	}
	if socket := document.daemonSocket(); socket != "" {
		values.DaemonSocket = socket
	}
	for name, override := range document.asciiuiTypes() {
		values.AsciiUITypes[name] = override
	}
	return values
}

// resolveRoot applies the three layers: command flag, then file value, then built-in default.
// File values resolve against the configuration directory; flag values resolve against the working
// directory, because that is where the user expressed them.
func resolveRoot(fileValue, defaultValue string, location Location, flagValue string) string {
	if flagValue != "" {
		return resolvePath(location.WorkingDir, flagValue)
	}
	raw := fileValue
	if raw == "" {
		raw = defaultValue
	}
	return resolvePath(location.ProjectDir, raw)
}

func resolvePath(base, raw string) string {
	if filepath.IsAbs(raw) {
		return filepath.Clean(raw)
	}
	return filepath.Join(base, raw)
}

// parseDuration reads a positive duration and falls back to the built-in default when the stored
// value is unusable; validation reports the stored value before a command runs.
func parseDuration(raw string, fallback time.Duration) time.Duration {
	parsed, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
