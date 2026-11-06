package editorial

import (
	"flag"
	"fmt"

	"github.com/Blue-Alien-Tec/clabok/internal/badge"
	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/config"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

type badgePayload struct {
	DryRun  bool   `json:"dryRun"`
	Book    string `json:"book"`
	Action  string `json:"action"`
	ID      string `json:"id,omitempty"`
	Label   string `json:"label,omitempty"`
	Message string `json:"message,omitempty"`
	Link    string `json:"link,omitempty"`
	Changed bool   `json:"changed"`
}

// DryRunResult marks the payload as a dry-run receipt.
func (result badgePayload) DryRunResult() bool { return result.DryRun }

func (result badgePayload) Text() string {
	change := "unchanged"
	if result.Changed {
		change = "changed"
	}
	line := fmt.Sprintf("%s badge %s: %s", result.Book, result.Action, change)
	if result.ID != "" {
		line += " " + result.ID
	}
	return line + "\n"
}

type badgeState struct {
	ID       string `json:"id"`
	Label    string `json:"label,omitempty"`
	Message  string `json:"message,omitempty"`
	Link     string `json:"link,omitempty"`
	Resolved bool   `json:"resolved"`
	Value    string `json:"value,omitempty"`
	Source   string `json:"source,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

type badgeView struct {
	DryRun bool         `json:"dryRun"`
	Book   string       `json:"book"`
	Badges []badgeState `json:"badges"`
}

// DryRunResult marks the payload as a dry-run receipt; a view writes nothing either way.
func (result badgeView) DryRunResult() bool { return true }

func badgeCommand() *cli.Command {
	var book, id, label, message, link string
	return &cli.Command{
		Name:     "badge",
		Summary:  "declare, remove or inspect the repository badges of the library README",
		Usage:    "pecia badge <add|remove|show> --book <id> [--id <detector>] [--label <label>] [--message <text>] [--link <url>]",
		Mutating: true,
		Flags: func(env *cli.Env) *flag.FlagSet {
			set := env.NewFlagSet("badge")
			set.StringVar(&book, "book", "", "book id")
			set.StringVar(&id, "id", "", "badge id or detector")
			set.StringVar(&label, "label", "", "badge label")
			set.StringVar(&message, "message", "", "explicit message that overrides the detector")
			set.StringVar(&link, "link", "", "link target of the badge")
			return set
		},
		Run: func(env *cli.Env, args []string) (*cli.Result, error) {
			if len(args) != 1 {
				return nil, cli.Usage("usage.invalid-arguments", "badge takes add, remove or show").
					WithHint("pecia badge add --book <id> --id go")
			}
			bookID, bookErr := requireBook(book)
			if bookErr != nil {
				return nil, bookErr
			}
			action := args[0]
			switch action {
			case "add":
				if id == "" {
					return nil, cli.Usage("usage.missing-argument", "--id is required")
				}
				service, serviceErr := newService(env)
				if serviceErr != nil {
					return nil, serviceErr
				}
				declared := model.Badge{ID: id, Label: label, Message: message, Link: link}
				receipt, err := service.SetBadge(bookID, declared, env.Globals.DryRun)
				if err != nil {
					return nil, cli.AsError(err)
				}
				return &cli.Result{Data: badgePayload{
					DryRun: env.Globals.DryRun, Book: string(bookID), Action: action,
					ID: id, Label: label, Message: message, Link: link, Changed: receipt.Changed,
				}}, nil
			case "remove":
				if id == "" {
					return nil, cli.Usage("usage.missing-argument", "--id is required")
				}
				if label != "" || message != "" || link != "" {
					return nil, cli.Usage("usage.invalid-arguments", "remove takes --book and --id only")
				}
				service, serviceErr := newService(env)
				if serviceErr != nil {
					return nil, serviceErr
				}
				receipt, err := service.RemoveBadge(bookID, id, env.Globals.DryRun)
				if err != nil {
					return nil, cli.AsError(err)
				}
				return &cli.Result{Data: badgePayload{
					DryRun: env.Globals.DryRun, Book: string(bookID), Action: action,
					ID: id, Changed: receipt.Changed,
				}}, nil
			case "show":
				if id != "" || label != "" || message != "" || link != "" {
					return nil, cli.Usage("usage.invalid-arguments", "show takes --book only")
				}
				service, serviceErr := newService(env)
				if serviceErr != nil {
					return nil, serviceErr
				}
				declared, err := service.Badges(bookID)
				if err != nil {
					return nil, cli.AsError(err)
				}
				values, configErr := config.Load(env)
				if configErr != nil {
					return nil, configErr
				}
				view := badgeView{
					DryRun: env.Globals.DryRun, Book: string(bookID),
					Badges: make([]badgeState, 0, len(declared)),
				}
				for _, item := range declared {
					state := badgeState{ID: item.ID, Label: item.Label, Message: item.Message, Link: item.Link}
					resolved, ok, reason := badge.Resolve(values.ProjectDir, item)
					if !ok {
						state.Reason = reason
						view.Badges = append(view.Badges, state)
						continue
					}
					state.Resolved = true
					state.Value = resolved.Message
					state.Source = resolved.Source
					if state.Label == "" {
						state.Label = resolved.Label
					}
					if state.Link == "" {
						state.Link = resolved.Link
					}
					view.Badges = append(view.Badges, state)
				}
				return &cli.Result{Data: view}, nil
			default:
				return nil, cli.Usage("usage.invalid-arguments",
					"unknown badge action %q; use add, remove or show", action)
			}
		},
	}
}
