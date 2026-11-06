package editorial

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/idspace"
	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/structure"
)

// Register adds the editorial command surface.
func Register(registry *cli.Registry) {
	registry.Add(proseCommand())
	registry.Add(regionCommand())
	registry.Add(asciiuiCommand())
	registry.Add(statusCommand())
	registry.Add(badgeCommand())
}

type payload struct {
	DryRun  bool     `json:"dryRun"`
	Book    string   `json:"book"`
	Kind    string   `json:"kind"`
	Target  string   `json:"target,omitempty"`
	Block   string   `json:"block,omitempty"`
	Parent  string   `json:"parent,omitempty"`
	Order   []string `json:"order,omitempty"`
	Changed bool     `json:"changed"`
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
	return line + "\n"
}

func proseCommand() *cli.Command {
	var book, node, text, file string
	return &cli.Command{
		Name:     "prose",
		Summary:  "set or clear the one-line index prose of a node",
		Usage:    "pecia prose <set|clear> --book <id> --node <id> [--prose <text> | --file <path|->]",
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
				return nil, cli.Usage("usage.invalid-arguments", "prose takes set or clear").
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
			switch args[0] {
			case "set":
				value, textErr := readText(text, file)
				if textErr != nil {
					return nil, textErr
				}
				prose = value
			case "clear":
				if text != "" || file != "" {
					return nil, cli.Usage("usage.invalid-arguments",
						"prose clear takes no --text or --file")
				}
			default:
				return nil, cli.Usage("usage.invalid-arguments",
					"unknown prose action %q; use set or clear", args[0])
			}
			service, serviceErr := newService(env)
			if serviceErr != nil {
				return nil, serviceErr
			}
			receipt, proseErr := service.SetProse(bookID, nodeID, prose, env.Globals.DryRun)
			if proseErr != nil {
				return nil, cli.AsError(proseErr)
			}
			return &cli.Result{Data: payloadOf(receipt, env.Globals.DryRun)}, nil
		},
	}
}

func regionCommand() *cli.Command {
	var book, kind, target, targetID, text, file string
	return &cli.Command{
		Name:     "region",
		Summary:  "author the footer and notes regions",
		Usage:    "pecia region <set|clear|notes|remove> --book <id> [--kind footer | --target node|block --id <id>] [--file <path|->]",
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
					"region takes set, clear, notes or remove").
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
				receipt, footerErr := service.SetFooter(bookID, body, env.Globals.DryRun)
				if footerErr != nil {
					return nil, cli.AsError(footerErr)
				}
				return &cli.Result{Data: payloadOf(receipt, env.Globals.DryRun)}, nil
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
				receipt, noteErr := service.AddNote(bookID, targetKind, id, body, env.Globals.DryRun)
				if noteErr != nil {
					return nil, cli.AsError(noteErr)
				}
				return &cli.Result{Data: payloadOf(receipt, env.Globals.DryRun)}, nil
			default:
				return nil, cli.Usage("usage.invalid-arguments",
					"unknown region action %q; use set, clear, notes or remove", args[0])
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
		return text, nil
	}
	if file == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", cli.Fail("editorial.read-failed", "%v", err)
		}
		return string(data), nil
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return "", cli.Fail("editorial.read-failed", "%v", err)
	}
	return string(data), nil
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
