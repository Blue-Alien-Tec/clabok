package grammar

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/config"
	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

// Register adds the grammar command surface.
func Register(registry *cli.Registry) {
	registry.Add(grammarCommand())
	registry.Add(termsCommand())
}

type checkPayload struct {
	DryRun     bool          `json:"dryRun"`
	Book       string        `json:"book,omitempty"`
	Block      string        `json:"block,omitempty"`
	Checked    bool          `json:"checked"`
	Reason     string        `json:"reason,omitempty"`
	Language   string        `json:"language,omitempty"`
	Degraded   string        `json:"degraded,omitempty"`
	Count      int           `json:"count"`
	Findings   []Finding     `json:"findings,omitempty"`
	Suppressed []Suppression `json:"suppressed,omitempty"`
	Warnings   []Warning     `json:"warnings,omitempty"`
}

// DryRunResult marks the payload as a dry-run receipt; a check writes nothing.
func (payload checkPayload) DryRunResult() bool { return payload.DryRun }

func (payload checkPayload) Text() string {
	var builder strings.Builder
	if payload.Degraded != "" {
		fmt.Fprintf(&builder, "grammar: %s\n", payload.Degraded)
	}
	for _, finding := range payload.Findings {
		fmt.Fprintf(&builder, "%d. line %d:%d %s: %s",
			finding.Index, finding.Line, finding.Column, finding.Rule, finding.Message)
		if len(finding.Replacements) == 1 {
			fmt.Fprintf(&builder, " -> %q", finding.Replacements[0])
		}
		builder.WriteByte('\n')
	}
	if !payload.Checked {
		fmt.Fprintf(&builder, "not checked")
		if payload.Reason != "" {
			fmt.Fprintf(&builder, " (%s)", payload.Reason)
		}
		return builder.String() + "\n"
	}
	fmt.Fprintf(&builder, "%d finding(s)\n", payload.Count)
	if len(payload.Suppressed) > 0 {
		counts := map[string]int{}
		scopes := make([]string, 0)
		for _, entry := range payload.Suppressed {
			if counts[entry.Layer] == 0 {
				scopes = append(scopes, entry.Layer)
			}
			counts[entry.Layer]++
		}
		sort.Strings(scopes)
		parts := make([]string, 0, len(scopes))
		for _, scope := range scopes {
			parts = append(parts, fmt.Sprintf("%s %d", scope, counts[scope]))
		}
		fmt.Fprintf(&builder, "%d suppressed by terms (%s)\n",
			len(payload.Suppressed), strings.Join(parts, ", "))
	}
	return builder.String()
}

type languagesPayload struct {
	DryRun       bool       `json:"dryRun"`
	Default      string     `json:"default"`
	DefaultKnown bool       `json:"defaultKnown"`
	Book         string     `json:"book,omitempty"`
	BookLanguage string     `json:"bookLanguage,omitempty"`
	Supported    bool       `json:"supported,omitempty"`
	Server       string     `json:"server"`
	Cached       bool       `json:"cached"`
	Stale        bool       `json:"stale,omitempty"`
	Fetched      string     `json:"fetched,omitempty"`
	Count        int        `json:"count"`
	Languages    []Language `json:"languages"`
}

// DryRunResult marks the payload as a dry-run receipt; the command writes nothing but the cache.
func (payload languagesPayload) DryRunResult() bool { return payload.DryRun }

func (payload languagesPayload) Text() string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "default language: %s", payload.Default)
	if payload.DefaultKnown {
		builder.WriteString(" (supported)")
	}
	builder.WriteByte('\n')
	if payload.Book != "" {
		fmt.Fprintf(&builder, "book %s: %s", payload.Book, payload.BookLanguage)
		if payload.Supported {
			builder.WriteString(" (supported)")
		}
		builder.WriteByte('\n')
	}
	for _, language := range payload.Languages {
		fmt.Fprintf(&builder, "%s  %s\n", language.Code, language.Name)
	}
	state := "server"
	if payload.Cached {
		state = "cached"
	}
	if payload.Stale {
		state += ", stale"
	}
	if payload.Fetched != "" {
		state += " (fetched " + payload.Fetched + ")"
	}
	fmt.Fprintf(&builder, "%d language(s), %s\n", payload.Count, state)
	return builder.String()
}

// languagesCommand prints the supported set, the configured default and an optional book language.
func languagesCommand(env *cli.Env, bookRaw string, refresh bool) (*cli.Result, error) {
	values, configErr := config.Load(env)
	if configErr != nil {
		return nil, configErr
	}
	life, _, lifeErr := lifecycle.FromEnv(env)
	if lifeErr != nil {
		return nil, lifeErr
	}
	client := NewClientTimeout(values.GrammarEndpoint, values.GrammarTimeout)
	languages, cached, stale, fetched := languageSnapshot(client, refresh)
	if len(languages) == 0 {
		return nil, cli.Fail("grammar.unreachable",
			"the grammar server cannot be reached and no language list is cached").
			WithHint("run `pecia grammar serve` once to fetch the list")
	}
	payload := languagesPayload{
		DryRun: env.Globals.DryRun, Default: values.GrammarLanguage,
		Server: values.GrammarEndpoint, Cached: cached, Stale: stale, Fetched: fetched,
		Count: len(languages), Languages: languages,
	}
	_, payload.DefaultKnown = findLanguage(languages, payload.Default)
	if bookRaw != "" {
		bookID, bookErr := requireBook(bookRaw)
		if bookErr != nil {
			return nil, bookErr
		}
		settings, settingsErr := BookSettings(life, bookID)
		if settingsErr != nil {
			return nil, cli.AsError(settingsErr)
		}
		payload.Book = string(bookID)
		payload.BookLanguage = Resolve(settings, values.GrammarLanguage)
		_, payload.Supported = findLanguage(languages, payload.BookLanguage)
	}
	return &cli.Result{Data: payload}, nil
}

func grammarCommand() *cli.Command {
	var book, block, file, raw, jar, finding, languageFlag string
	var allSafe, requireCheck, refresh bool
	var expect int
	return &cli.Command{
		Name:     "grammar",
		Summary:  "check prose, apply findings and manage the LanguageTool server",
		Usage:    "pecia grammar <check|apply|languages|serve|server-stop|server-info> ...",
		Mutating: true,
		Flags: func(env *cli.Env) *flag.FlagSet {
			set := env.NewFlagSet("grammar")
			set.StringVar(&book, "book", "", "book id")
			set.StringVar(&block, "block", "", "block id")
			set.StringVar(&file, "file", "", "file to check, or - for stdin")
			set.StringVar(&raw, "raw", "", "raw text to check")
			set.StringVar(&jar, "jar", "", "LanguageTool server jar")
			set.BoolVar(&allSafe, "all-safe", false, "apply all findings with one replacement")
			set.BoolVar(&requireCheck, "require-check", false, "fail when the check cannot complete")
			set.StringVar(&languageFlag, "language", "", "check language id, or auto for --file and --raw")
			set.StringVar(&finding, "finding", "", "1-based finding indices")
			set.IntVar(&expect, "expect", -1, "expected number of replacements")
			set.BoolVar(&refresh, "refresh", false, "refresh the cached language list")
			return set
		},
		Run: func(env *cli.Env, args []string) (*cli.Result, error) {
			if len(args) == 1 {
				values, configErr := config.Load(env)
				if configErr != nil {
					return nil, configErr
				}
				switch args[0] {
				case "apply":
					if file != "" || raw != "" {
						return nil, cli.Usage("usage.invalid-arguments",
							"apply reads a block, not --file or --raw")
					}
					return runApply(env, book, block, finding, allSafe, expect, jar)
				case "serve":
					if env.Globals.DryRun {
						info, _ := Info(values.GrammarEndpoint)
						info.DryRun = true
						return &cli.Result{Data: info}, nil
					}
					info, serveErr := Serve(values.GrammarEndpoint, jar)
					if serveErr != nil {
						return nil, cli.AsError(serveErr)
					}
					return &cli.Result{Data: info}, nil
				case "server-stop":
					if env.Globals.DryRun {
						info, _ := Info(values.GrammarEndpoint)
						info.DryRun = true
						return &cli.Result{Data: info}, nil
					}
					info, stopErr := Stop(values.GrammarEndpoint)
					if stopErr != nil {
						return nil, cli.AsError(stopErr)
					}
					return &cli.Result{Data: info}, nil
				case "server-info":
					info, infoErr := Info(values.GrammarEndpoint)
					if infoErr != nil {
						return nil, cli.AsError(infoErr)
					}
					interval, cap := Watchdog(values.GrammarTimeout)
					info.Timeout = interval.String()
					info.Cap = cap.String()
					info.Language = values.GrammarLanguage
					client := NewClientTimeout(values.GrammarEndpoint, values.GrammarTimeout)
					info.Languages, info.LanguagesCached, info.LanguagesStale,
						info.LanguagesFetched = languageSnapshot(client, false)
					info.DryRun = env.Globals.DryRun
					return &cli.Result{Data: info}, nil
				case "languages":
					return languagesCommand(env, book, refresh)
				}
			}
			if len(args) != 1 || args[0] != "check" {
				return nil, cli.Usage("usage.invalid-arguments",
					"grammar takes check, apply, languages, serve, server-stop or server-info").
					WithHint("pecia grammar check --book <id> --block <id>")
			}
			sources := 0
			if book != "" {
				sources++
			}
			if file != "" {
				sources++
			}
			if raw != "" {
				sources++
			}
			if sources != 1 {
				return nil, cli.Usage("usage.invalid-arguments",
					"pass exactly one of --book, --file or --raw")
			}
			values, configErr := config.Load(env)
			if configErr != nil {
				return nil, configErr
			}
			life, layout, lifeErr := lifecycle.FromEnv(env)
			if lifeErr != nil {
				return nil, lifeErr
			}
			payload := checkPayload{DryRun: env.Globals.DryRun}
			content := raw
			language := strings.TrimSpace(values.GrammarLanguage)
			if book != "" {
				if strings.TrimSpace(languageFlag) != "" {
					return nil, cli.Usage("usage.invalid-arguments",
						"--language is only for --file and --raw; a book uses its declared language").
						WithHint("pecia book set <book> language <id>")
				}
				bookID, bookErr := requireBook(book)
				if bookErr != nil {
					return nil, bookErr
				}
				blockID, blockErr := requireBlock(block)
				if blockErr != nil {
					return nil, blockErr
				}
				read, readErr := readBlock(env, bookID, blockID)
				if readErr != nil {
					return nil, cli.AsError(readErr)
				}
				content = read
				settings, settingsErr := BookSettings(life, bookID)
				if settingsErr != nil {
					return nil, cli.AsError(settingsErr)
				}
				language = Resolve(settings, language)
				payload.Book = string(bookID)
				payload.Block = blockID.String()
			} else if file != "" {
				read, readErr := readFile(file)
				if readErr != nil {
					return nil, readErr
				}
				content = read
			}
			if strings.TrimSpace(languageFlag) != "" {
				language = strings.TrimSpace(languageFlag)
			}
			blockID := model.ZeroID
			if block != "" {
				blockID, _ = model.ParseID(block)
			}
			gate := NewGateTimeout(values.GrammarEndpoint, ParsePolicy(values.GrammarPolicy),
				TermSource{GlobalPath: layout.Terms(), Dir: layout.TermsDir()},
				values.GrammarTimeout)
			gate.AdHoc = book == ""
			gate.Start = func(endpoint string) error {
				_, startErr := Serve(endpoint, jar)
				return startErr
			}
			report := gate.Evaluate(context.Background(), blockID, content, language)
			payload.Checked = report.Checked
			payload.Reason = report.Reason
			payload.Language = report.Language
			payload.Degraded = report.Degraded
			payload.Warnings = report.Warnings()
			payload.Count = len(report.Findings)
			payload.Findings = report.Findings
			payload.Suppressed = report.Suppressed
			if report.Unchecked() && requireCheck {
				return nil, cli.Fail("grammar.check-skipped", "%s", report.Degraded).
					WithHint("--require-check makes an incomplete check a failure").
					WithDetails(report)
			}
			return &cli.Result{Data: payload}, nil
		},
	}
}

func requireBook(raw string) (model.BookID, *cli.Error) {
	if raw == "" {
		return "", cli.Usage("usage.missing-argument", "--book is required")
	}
	id := model.BookID(raw)
	if !id.Valid() {
		return "", cli.Usage("usage.invalid-value", "invalid book id %q", raw)
	}
	return id, nil
}

func requireBlock(raw string) (model.ID, *cli.Error) {
	id, err := model.ParseID(raw)
	if err != nil || id.IsZero() {
		return model.ZeroID, cli.Usage("usage.invalid-value", "invalid block id %q", raw)
	}
	return id, nil
}

func readBlock(env *cli.Env, book model.BookID, block model.ID) (string, error) {
	life, _, err := lifecycle.FromEnv(env)
	if err != nil {
		return "", err
	}
	paths, openErr := life.RequireOpen(book)
	if openErr != nil {
		return "", openErr
	}
	data, readErr := os.ReadFile(filepath.Join(paths.Dir, lifecycle.ManifestMember))
	if readErr != nil {
		return "", cli.Fail("grammar.read-failed", "%v", readErr)
	}
	manifest, decodeErr := model.Decode(data)
	if decodeErr != nil {
		return "", cli.AsError(decodeErr)
	}
	if !manifest.IsPlaced(block) {
		return "", cli.Reject("grammar.unknown-block", "block %s is not placed", block)
	}
	body, bodyErr := os.ReadFile(filepath.Join(paths.BlocksDir, block.String()+".md"))
	if bodyErr != nil && !errors.Is(bodyErr, os.ErrNotExist) {
		return "", cli.Fail("grammar.read-failed", "%v", bodyErr)
	}
	return string(body), nil
}

func readFile(path string) (string, *cli.Error) {
	var data []byte
	var err error
	if path == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return "", cli.Fail("grammar.read-failed", "%v", err)
	}
	return string(data), nil
}
