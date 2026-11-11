package editorial

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/Blue-Alien-Tec/clabok/internal/asciiui"
	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/structure"
	"github.com/Blue-Alien-Tec/clabok/internal/textinput"
)

type layoutPayload struct {
	DryRun  bool   `json:"dryRun"`
	Book    string `json:"book"`
	Action  string `json:"action"`
	Block   string `json:"block,omitempty"`
	Parent  string `json:"parent,omitempty"`
	Changed bool   `json:"changed"`
}

// DryRunResult marks the payload as a dry-run receipt.
func (result layoutPayload) DryRunResult() bool { return result.DryRun }

func (result layoutPayload) Text() string {
	state := "unchanged"
	if result.Changed {
		state = "changed"
	}
	line := fmt.Sprintf("%s asciiui %s: %s", result.Book, result.Action, state)
	if result.Block != "" {
		line += " " + result.Block
	}
	return line + "\n"
}

func asciiuiCommand() *cli.Command {
	var book, parent, block, file string
	actions := []cli.Action{
		{Name: "add", Summary: "add a layout block from a spec file",
			Reads: []string{"book", "parent", "file"}},
		{Name: "set", Summary: "replace the spec of a layout block",
			Reads: []string{"book", "block", "file"}},
		{Name: "remove", Summary: "remove a layout block",
			Reads: []string{"book", "block"}},
	}
	return &cli.Command{
		Name:     "asciiui",
		Summary:  "author an ASCII UI layout on a locked UILayout block",
		Usage:    "pecia asciiui <add|set|remove> --book <id> [--parent <node> --file <path|->] [--block <id> [--file <path|->]]",
		Actions:  actions,
		Mutating: true,
		Flags: func(env *cli.Env) *flag.FlagSet {
			set := env.NewFlagSet("asciiui")
			set.StringVar(&book, "book", "", "book id")
			set.StringVar(&parent, "parent", "", "target node for add")
			set.StringVar(&block, "block", "", "layout block id for set and remove")
			set.StringVar(&file, "file", "", "spec file, or - for stdin")
			return set
		},
		Run: func(env *cli.Env, args []string) (*cli.Result, error) {
			if len(args) != 1 {
				return nil, cli.Usage("usage.invalid-arguments",
					"asciiui takes %s", cli.ActionList(actions)).
					WithHint("pecia asciiui add --book <id> --parent <node> --file <path>")
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
			case "add":
				if block != "" {
					return nil, cli.Usage("usage.invalid-arguments", "add takes --parent, not --block")
				}
				parentID, idErr := requireID(parent, "parent")
				if idErr != nil {
					return nil, idErr
				}
				spec, specErr := readSpec(file)
				if specErr != nil {
					return nil, specErr
				}
				receipt, addErr := service.AddLayoutBlock(bookID, parentID, spec, env.Globals.DryRun)
				if addErr != nil {
					return nil, cli.AsError(addErr)
				}
				return &cli.Result{Data: layoutPayloadOf(receipt, "add", parentID, env.Globals.DryRun)}, nil
			case "set":
				if parent != "" {
					return nil, cli.Usage("usage.invalid-arguments", "set takes --block, not --parent")
				}
				blockID, idErr := requireID(block, "block")
				if idErr != nil {
					return nil, idErr
				}
				spec, specErr := readSpec(file)
				if specErr != nil {
					return nil, specErr
				}
				receipt, setErr := service.SetLayout(bookID, blockID, spec, env.Globals.DryRun)
				if setErr != nil {
					return nil, cli.AsError(setErr)
				}
				return &cli.Result{Data: layoutPayloadOf(receipt, "set", model.ZeroID, env.Globals.DryRun)}, nil
			case "remove":
				if parent != "" || file != "" {
					return nil, cli.Usage("usage.invalid-arguments", "remove takes --block only")
				}
				blockID, idErr := requireID(block, "block")
				if idErr != nil {
					return nil, idErr
				}
				receipt, removeErr := service.RemoveLayout(bookID, blockID, env.Globals.DryRun)
				if removeErr != nil {
					return nil, cli.AsError(removeErr)
				}
				return &cli.Result{Data: layoutPayloadOf(receipt, "remove", model.ZeroID, env.Globals.DryRun)}, nil
			default:
				return nil, cli.Usage("usage.invalid-arguments",
					"unknown asciiui action %q; use %s", args[0], cli.ActionList(actions))
			}
		},
	}
}

// readSpec reads and validates a spec before anything is written.
func readSpec(file string) (string, *cli.Error) {
	if file == "" {
		return "", cli.Usage("usage.missing-argument", "--file is required").
			WithHint("pass the spec with --file <path|->")
	}
	var data []byte
	var err error
	if file == "-" {
		data, err = textinput.ReadStdin(os.Stdin)
	} else {
		data, err = textinput.ReadFile(file)
	}
	if err != nil {
		var decodeErr *textinput.Error
		if errors.As(err, &decodeErr) {
			return "", cli.AsError(decodeErr)
		}
		return "", cli.Fail("asciiui.read-failed", "%v", err)
	}
	if parseErr := asciiui.Validate(string(data)); parseErr != nil {
		return "", cli.AsError(parseErr)
	}
	return string(data), nil
}

func layoutPayloadOf(receipt structure.Receipt, action string, parent model.ID, dryRun bool) layoutPayload {
	result := layoutPayload{
		DryRun: dryRun, Book: string(receipt.Book), Action: action, Changed: receipt.Changed,
	}
	if !receipt.Block.IsZero() {
		result.Block = receipt.Block.String()
	} else if !receipt.Target.IsZero() {
		result.Block = receipt.Target.String()
	}
	if !parent.IsZero() {
		result.Parent = parent.String()
	}
	return result
}
