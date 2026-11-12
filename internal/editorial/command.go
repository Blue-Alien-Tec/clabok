package editorial

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/config"
	"github.com/Blue-Alien-Tec/clabok/internal/grammar"
	"github.com/Blue-Alien-Tec/clabok/internal/idspace"
	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/structure"
	"github.com/Blue-Alien-Tec/clabok/internal/textinput"
)

// Register adds the editorial command surface.
func Register(registry *cli.Registry) {
	registry.Add(proseCommand())
	registry.Add(regionCommand())
	registry.Add(asciiuiCommand())
	registry.Add(statusCommand())
	registry.Add(partCommand())
	registry.Add(badgeCommand())
	registry.Add(layoutCommand())
}

type payload struct {
	DryRun   bool              `json:"dryRun"`
	Book     string            `json:"book"`
	Kind     string            `json:"kind"`
	Target   string            `json:"target,omitempty"`
	Block    string            `json:"block,omitempty"`
	Parent   string            `json:"parent,omitempty"`
	Order    []string          `json:"order,omitempty"`
	Changed  bool              `json:"changed"`
	Grammar  *grammar.Report   `json:"grammar,omitempty"`
	Warnings []grammar.Warning `json:"warnings,omitempty"`
}

// DryRunResult marks the payload as a dry-run receipt.
func (result payload) DryRunResult() bool { return result.DryRun }

func (result payload) Text() string {
	state := "unchanged"
	if result.Changed {
		state = "changed"
	}
	line := fmt.Sprintf("%s %s: %s", result.Book, result.Kind, state)
	if result.Target != "" {
		line += " " + result.Target
	}
	if result.Grammar != nil {
		line += grammarReportText(*result.Grammar)
	}
	return line + "\n"
}

// evaluateProse runs the grammar gate for an authored prose body. Every prose write path calls it,
// so there is no way around the check; a server that cannot be started degrades to a skip report.
func evaluateProse(env *cli.Env, book model.BookID, text string) (grammar.Report, *cli.Error) {
	if strings.TrimSpace(text) == "" {
		return grammar.Report{}, nil
	}
	values, configErr := config.Load(env)
	if configErr != nil {
		return grammar.Report{}, configErr
	}
	life, layout, lifeErr := lifecycle.FromEnv(env)
	if lifeErr != nil {
		return grammar.Report{}, lifeErr
	}
	settings, settingsErr := grammar.BookSettings(life, book)
	if settingsErr != nil {
		return grammar.Report{}, cli.AsError(settingsErr)
	}
	gate := grammar.NewGateTimeout(values.GrammarEndpoint,
		grammar.ParsePolicy(values.GrammarPolicy),
		grammar.TermSource{GlobalPath: layout.Terms(), Dir: layout.TermsDir()},
		values.GrammarTimeout)
	language := grammar.Resolve(settings, values.GrammarLanguage)
	return gate.Evaluate(context.Background(), 0, text, language), nil
}

func proseRejection(report grammar.Report) *cli.Error {
	return cli.Reject("body.grammar-blocked", "the grammar check found %d finding(s)",
		len(report.Findings)).
		WithHint("fix the findings or set grammar.policy to warn in clabok.json").
		WithDetails(report)
}

func grammarPtr(report grammar.Report) *grammar.Report {
	if !report.Checked && report.Degraded == "" && len(report.Findings) == 0 {
		return nil
	}
	return &report
}

func grammarReportText(report grammar.Report) string {
	var builder strings.Builder
	if report.Degraded != "" {
		fmt.Fprintf(&builder, "\ngrammar: %s", report.Degraded)
	}
	if len(report.Findings) > 0 {
		fmt.Fprintf(&builder, "\ngrammar: %d finding(s)", len(report.Findings))
		for _, finding := range report.Findings {
			fmt.Fprintf(&builder, "\n  %d. line %d:%d %s: %s",
				finding.Index, finding.Line, finding.Column, finding.Rule, finding.Message)
		}
	}
	return builder.String()
}

func proseCommand() *cli.Command {
	var book, node, text, file string
	actions := []cli.Action{
		{Name: "set", Summary: "set the index prose of a chapter",
			Reads: []string{"book", "node", "prose", "file"}},
		{Name: "clear", Summary: "clear the index prose of a chapter",
			Reads: []string{"book", "node"}},
	}
	return &cli.Command{
		Name:     "prose",
		Summary:  "set or clear the one-line index prose of a node",
		Usage:    "pecia prose <set|clear> --book <id> --node <id> [--prose <text> | --file <path|->]",
		Actions:  actions,
		Mutating: true,
		Flags: func(env *cli.Env) *flag.FlagSet {
			set := env.NewFlagSet("prose")
			set.StringVar(&book, "book", "", "book id")
			set.StringVar(&node, "node", "", "target node id")
			set.StringVar(&text, "prose", "", "prose text")
			set.StringVar(&file, "file", "", "prose file, or - for stdin")
			return set
		},
		Run: func(env *cli.Env, args []string) (*cli.Result, error) {
			if len(args) != 1 {
				return nil, cli.Usage("usage.invalid-arguments",
					"prose takes %s", cli.ActionList(actions)).
					WithHint("pecia prose set --book <id> --node <id> --prose <text>")
			}
			bookID, bookErr := requireBook(book)
			if bookErr != nil {
				return nil, bookErr
			}
			nodeID, idErr := requireID(node, "node")
			if idErr != nil {
				return nil, idErr
			}
			var prose string
			var report grammar.Report
			switch args[0] {
			case "set":
				value, textErr := readText(text, file)
				if textErr != nil {
					return nil, textErr
				}
				prose = value
				report, textErr = evaluateProse(env, bookID, prose)
				if textErr != nil {
					return nil, textErr
				}
				if report.Blocked() {
					return nil, proseRejection(report)
				}
			case "clear":
				if text != "" || file != "" {
					return nil, cli.Usage("usage.invalid-arguments",
						"prose clear takes no --text or --file")
				}
			default:
				return nil, cli.Usage("usage.invalid-arguments",
					"unknown prose action %q; use %s", args[0], cli.ActionList(actions))
			}
			service, serviceErr := newService(env)
			if serviceErr != nil {
				return nil, serviceErr
			}
			receipt, proseErr := service.SetProse(bookID, nodeID, prose, env.Globals.DryRun)
			if proseErr != nil {
				return nil, cli.AsError(proseErr)
			}
			result := payloadOf(receipt, env.Globals.DryRun)
			result.Grammar = grammarPtr(report)
			result.Warnings = report.Warnings()
			return &cli.Result{Data: result}, nil
		},
	}
}

func regionCommand() *cli.Command {
	var book, kind, target, targetID, text, file string
	actions := []cli.Action{
		{Name: "set", Summary: "write a region body",
			Reads: []string{"book", "kind", "target", "id", "body", "file"}},
		{Name: "clear", Summary: "clear a region body",
			Reads: []string{"book", "kind", "target", "id"}},
		{Name: "notes", Summary: "add a note to a node or block",
			Reads: []string{"book", "target", "id", "body", "file"}},
		{Name: "remove", Summary: "remove a note from a node or block",
			Reads: []string{"book", "target", "id"}},
	}
	return &cli.Command{
		Name:     "region",
		Summary:  "author the footer and notes regions",
		Usage:    "pecia region <set|clear|notes|remove> --book <id> [--kind footer | --target node|block --id <id>] [--file <path|->]",
		Actions:  actions,
		Mutating: true,
		Flags: func(env *cli.Env) *flag.FlagSet {
			set := env.NewFlagSet("region")
			set.StringVar(&book, "book", "", "book id")
			set.StringVar(&kind, "kind", "", "region kind (footer)")
			set.StringVar(&target, "target", "", "notes target kind (node or block)")
			set.StringVar(&targetID, "id", "", "notes target id")
			set.StringVar(&text, "body", "", "body text")
			set.StringVar(&file, "file", "", "body file, or - for stdin")
			return set
		},
		Run: func(env *cli.Env, args []string) (*cli.Result, error) {
			if len(args) != 1 {
				return nil, cli.Usage("usage.invalid-arguments",
					"region takes %s", cli.ActionList(actions)).
					WithHint("pecia region notes --book <id> --target node --id <id> --file <path>")
			}
			bookID, bookErr := requireBook(book)
			if bookErr != nil {
				return nil, bookErr
			}
			service, serviceErr := newService(env)
			if serviceErr != nil {
				return nil, serviceErr
			}
			switch args[0] {
			case "set", "clear":
				if strings.TrimSpace(kind) != string(model.RegionFooter) {
					return nil, cli.Usage("usage.invalid-arguments", "the footer is the only region set kind")
				}
				if target != "" || targetID != "" {
					return nil, cli.Usage("usage.invalid-arguments", "the footer takes no --target or --id")
				}
				if args[0] == "clear" {
					if text != "" || file != "" {
						return nil, cli.Usage("usage.invalid-arguments", "region clear takes no body")
					}
					receipt, clearErr := service.ClearFooter(bookID, env.Globals.DryRun)
					if clearErr != nil {
						return nil, cli.AsError(clearErr)
					}
					return &cli.Result{Data: payloadOf(receipt, env.Globals.DryRun)}, nil
				}
				body, bodyErr := readText(text, file)
				if bodyErr != nil {
					return nil, bodyErr
				}
				report, gateErr := evaluateProse(env, bookID, body)
				if gateErr != nil {
					return nil, gateErr
				}
				if report.Blocked() {
					return nil, proseRejection(report)
				}
				receipt, footerErr := service.SetFooter(bookID, body, env.Globals.DryRun)
				if footerErr != nil {
					return nil, cli.AsError(footerErr)
				}
				result := payloadOf(receipt, env.Globals.DryRun)
				result.Grammar = grammarPtr(report)
				result.Warnings = report.Warnings()
				return &cli.Result{Data: result}, nil
			case "notes", "remove":
				targetKind, kindErr := parseTargetKind(target)
				if kindErr != nil {
					return nil, kindErr
				}
				id, idErr := requireID(targetID, "target")
				if idErr != nil {
					return nil, idErr
				}
				if args[0] == "remove" {
					if text != "" || file != "" {
						return nil, cli.Usage("usage.invalid-arguments", "region remove takes no body")
					}
					receipt, removeErr := service.RemoveNote(bookID, targetKind, id, env.Globals.DryRun)
					if removeErr != nil {
						return nil, cli.AsError(removeErr)
					}
					return &cli.Result{Data: payloadOf(receipt, env.Globals.DryRun)}, nil
				}
				body, bodyErr := readText(text, file)
				if bodyErr != nil {
					return nil, bodyErr
				}
				report, gateErr := evaluateProse(env, bookID, body)
				if gateErr != nil {
					return nil, gateErr
				}
				if report.Blocked() {
					return nil, proseRejection(report)
				}
				receipt, noteErr := service.AddNote(bookID, targetKind, id, body, env.Globals.DryRun)
				if noteErr != nil {
					return nil, cli.AsError(noteErr)
				}
				result := payloadOf(receipt, env.Globals.DryRun)
				result.Grammar = grammarPtr(report)
				result.Warnings = report.Warnings()
				return &cli.Result{Data: result}, nil
			default:
				return nil, cli.Usage("usage.invalid-arguments",
					"unknown region action %q; use %s", args[0], cli.ActionList(actions))
			}
		},
	}
}

func newService(env *cli.Env) (*structure.Service, *cli.Error) {
	life, layout, err := lifecycle.FromEnv(env)
	if err != nil {
		return nil, err
	}
	return structure.New(life, idspace.New(life, layout)), nil
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

func requireID(raw, name string) (model.ID, *cli.Error) {
	id, err := model.ParseID(raw)
	if err != nil || id.IsZero() {
		return model.ZeroID, cli.Usage("usage.invalid-value", "invalid %s id %q", name, raw)
	}
	return id, nil
}

func parseTargetKind(raw string) (model.TargetKind, *cli.Error) {
	switch model.TargetKind(strings.TrimSpace(raw)) {
	case model.TargetNode:
		return model.TargetNode, nil
	case model.TargetBlock:
		return model.TargetBlock, nil
	default:
		return "", cli.Usage("usage.invalid-value",
			"--target expects node or block, got %q", raw)
	}
}

func readText(text, file string) (string, *cli.Error) {
	if text != "" && file != "" {
		return "", cli.Usage("usage.invalid-arguments", "use --text or --file, not both")
	}
	if text == "" && file == "" {
		return "", cli.Usage("usage.missing-argument", "a body is required").
			WithHint("pass the text with the command's text flag or --file <path|->")
	}
	if text != "" {
		decoded, decodeErr := textinput.Decode([]byte(text))
		if decodeErr != nil {
			return "", inputError(decodeErr)
		}
		return string(decoded), nil
	}
	var data []byte
	var err error
	if file == "-" {
		data, err = textinput.ReadStdin(os.Stdin)
	} else {
		data, err = textinput.ReadFile(file)
	}
	if err != nil {
		return "", inputError(err)
	}
	return string(data), nil
}

// inputError maps a decode failure to its code and an I/O failure to the command's read code.
func inputError(err error) *cli.Error {
	var decodeErr *textinput.Error
	if errors.As(err, &decodeErr) {
		return cli.AsError(decodeErr)
	}
	return cli.Fail("editorial.read-failed", "%v", err)
}

func payloadOf(receipt structure.Receipt, dryRun bool) payload {
	result := payload{
		DryRun: dryRun, Book: string(receipt.Book), Kind: receipt.Kind,
		Changed: receipt.Changed,
	}
	if !receipt.Target.IsZero() {
		result.Target = receipt.Target.String()
	}
	if !receipt.Block.IsZero() {
		result.Block = receipt.Block.String()
	}
	if !receipt.Parent.IsZero() {
		result.Parent = receipt.Parent.String()
	}
	for _, id := range receipt.Order {
		result.Order = append(result.Order, id.String())
	}
	return result
}
