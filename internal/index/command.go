package index

import (
	"flag"
	"fmt"
	"strings"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

// Warning is a per-book condition of an index run.
type Warning struct {
	Code    string `json:"code"`
	Book    string `json:"book,omitempty"`
	Message string `json:"message"`
}

type payload struct {
	Action   string    `json:"action"`
	DryRun   bool      `json:"dryRun"`
	Written  bool      `json:"written"`
	Books    []Status  `json:"books"`
	Warnings []Warning `json:"warnings"`
}

// Text renders the human form.
func (result payload) Text() string {
	var builder strings.Builder
	for _, status := range result.Books {
		fmt.Fprintf(&builder, "%s: %s\n", status.Book, status.State)
	}
	for _, warning := range result.Warnings {
		fmt.Fprintf(&builder, "warning: %s: %s\n", warning.Code, warning.Message)
	}
	return builder.String()
}

// Register adds the index command.
func Register(registry *cli.Registry) {
	var book string
	var all, write, verify bool
	actions := []cli.Action{
		{Name: "status", Summary: "report the index state of a book or the library",
			Reads: []string{"book", "all", "verify"}},
		{Name: "build", Summary: "build the search index",
			Reads: []string{"book", "all", "write"}},
		{Name: "clear", Summary: "remove the search index",
			Reads: []string{"book", "all", "write"}},
	}
	registry.Add(&cli.Command{
		Name:    "index",
		Summary: "build, inspect or clear the derived search index",
		Usage:   "pecia index <status|build|clear> [--book <id> | --all] [--write] [--verify]",
		Actions: actions,
		Flags: func(env *cli.Env) *flag.FlagSet {
			set := env.NewFlagSet("index")
			set.StringVar(&book, "book", "", "book id")
			set.BoolVar(&all, "all", false, "every registered book")
			set.BoolVar(&write, "write", false, "write the index")
			set.BoolVar(&verify, "verify", false, "status: compare the full container hash")
			return set
		},
		Run: func(env *cli.Env, args []string) (*cli.Result, error) {
			action := "status"
			if len(args) == 1 {
				action = args[0]
			} else if len(args) > 1 {
				return nil, cli.Usage("usage.unexpected-argument", "index takes one action")
			}
			switch action {
			case "status", "build", "clear":
			default:
				return nil, cli.Usage("usage.invalid-value",
					"unknown index action %q; use %s", action, cli.ActionList(actions))
			}
			if verify && action != "status" {
				return nil, cli.Usage("usage.invalid-value", "--verify applies to index status")
			}
			if book != "" && all {
				return nil, cli.Reject("index.scope-conflict", "use either --book or --all")
			}
			if book == "" && !all {
				if action == "status" {
					all = true
				} else {
					return nil, cli.Reject("index.scope-required", "--book or --all is required")
				}
			}
			life, layout, envErr := lifecycle.FromEnv(env)
			if envErr != nil {
				return nil, envErr
			}
			registryValue, registryErr := store.LoadRegistry(layout.Registry())
			if registryErr != nil {
				return nil, cli.Fail("index.read-failed", "%v", registryErr)
			}
			service := New(life, registryValue, layout.Root)
			ids := make([]model.BookID, 0, len(registryValue.Books))
			if book != "" {
				if !model.BookID(book).Valid() {
					return nil, cli.Usage("usage.invalid-value", "invalid book id %q", book)
				}
				if _, exists := registryValue.Find(model.BookID(book)); !exists {
					return nil, cli.Reject("index.unknown-book", "book %q is not registered", book)
				}
				ids = append(ids, model.BookID(book))
			} else {
				for _, entry := range registryValue.Books {
					ids = append(ids, entry.ID)
				}
			}
			result := payload{
				Action: action, DryRun: !write && action != "status",
				Books: make([]Status, 0, len(ids)), Warnings: []Warning{},
			}
			for _, id := range ids {
				var status Status
				var err error
				if verify {
					status, err = service.StatusVerified(id)
				} else {
					status, err = service.Status(id)
				}
				if err != nil {
					return nil, cli.Fail("index.read-failed", "%v", err)
				}
				if status.State == "open" {
					if action == "build" {
						result.Warnings = append(result.Warnings, Warning{
							Code: "index.open-book", Book: string(id),
							Message: "a closed book is required to build the index",
						})
						continue
					}
					result.Books = append(result.Books, status)
					continue
				}
				switch action {
				case "build":
					if !write {
						result.Books = append(result.Books, status)
						continue
					}
					built, buildErr := service.Build(id)
					if buildErr != nil {
						return nil, cli.AsError(buildErr)
					}
					built.State = "built"
					result.Books = append(result.Books, built)
					result.Written = true
				case "clear":
					if !write {
						result.Books = append(result.Books, status)
						continue
					}
					if clearErr := service.Clear(id); clearErr != nil {
						return nil, cli.AsError(clearErr)
					}
					status.State = "cleared"
					result.Books = append(result.Books, status)
					result.Written = true
				default:
					result.Books = append(result.Books, status)
				}
			}
			return &cli.Result{Data: result}, nil
		},
	})
}
