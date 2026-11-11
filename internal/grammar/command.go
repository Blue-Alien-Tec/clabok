package grammar

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	DryRun   bool      `json:"dryRun"`
	Book     string    `json:"book,omitempty"`
	Block    string    `json:"block,omitempty"`
	Checked  bool      `json:"checked"`
	Reason   string    `json:"reason,omitempty"`
	Degraded string    `json:"degraded,omitempty"`
	Count    int       `json:"count"`
	Findings []Finding `json:"findings,omitempty"`
	Warnings []Warning `json:"warnings,omitempty"`
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
	return builder.String()
}

func grammarCommand() *cli.Command {
	var book, block, file, raw, jar, finding string
	var allSafe, requireCheck bool
	var expect int
	return &cli.Command{
		Name:     "grammar",
		Summary:  "check prose, apply findings and manage the LanguageTool server",
		Usage:    "pecia grammar <check|apply|serve|server-stop|server-info> ...",
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
			set.StringVar(&finding, "finding", "", "1-based finding indices")
			set.IntVar(&expect, "expect", -1, "expected number of replacements")
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
					info.DryRun = env.Globals.DryRun
					return &cli.Result{Data: info}, nil
				}
			}
			if len(args) != 1 || args[0] != "check" {
				return nil, cli.Usage("usage.invalid-arguments",
					"grammar takes check, apply, serve, server-stop or server-info").
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
			_, layout, lifeErr := lifecycle.FromEnv(env)
			if lifeErr != nil {
				return nil, lifeErr
			}
			payload := checkPayload{DryRun: env.Globals.DryRun}
			content := raw
			if book != "" {
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
				payload.Book = string(bookID)
				payload.Block = blockID.String()
			} else if file != "" {
				read, readErr := readFile(file)
				if readErr != nil {
					return nil, readErr
				}
				content = read
			}
			blockID := model.ZeroID
			if block != "" {
				blockID, _ = model.ParseID(block)
			}
			gate := NewGateTimeout(values.GrammarEndpoint, ParsePolicy(values.GrammarPolicy),
				ReadTerms(layout.Terms()), values.GrammarTimeout)
			gate.Start = func(endpoint string) error {
				_, startErr := Serve(endpoint, jar)
				return startErr
			}
			report := gate.Evaluate(context.Background(), blockID, content)
			payload.Checked = report.Checked
			payload.Reason = report.Reason
			payload.Degraded = report.Degraded
			payload.Warnings = report.Warnings()
			payload.Count = len(report.Findings)
			payload.Findings = report.Findings
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
