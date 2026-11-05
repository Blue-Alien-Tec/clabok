package temp

import (
	"flag"
	"fmt"
	"strings"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/config"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

type filePayload struct {
	Path string `json:"path"`
	Book string `json:"book,omitempty"`
	Size int64  `json:"size"`
}

type payload struct {
	DryRun  bool          `json:"dryRun"`
	Deleted int           `json:"deleted"`
	Files   []filePayload `json:"files"`
}

// DryRunResult marks the payload as a dry-run receipt.
func (payload payload) DryRunResult() bool { return payload.DryRun }

func (payload payload) Text() string {
	var builder strings.Builder
	for _, file := range payload.Files {
		fmt.Fprintf(&builder, "%s  %d byte(s)\n", file.Path, file.Size)
	}
	state := "listed"
	if !payload.DryRun {
		state = fmt.Sprintf("deleted %d", payload.Deleted)
	}
	fmt.Fprintf(&builder, "%d file(s) %s\n", len(payload.Files), state)
	return builder.String()
}

// Register adds the flush command.
func Register(registry *cli.Registry) {
	var book string
	var all, write bool
	registry.Add(&cli.Command{
		Name:     "flush",
		Summary:  "list or delete transient authoring inputs in .temp",
		Usage:    "pecia flush --book <id> [--write] | --all [--write]",
		Mutating: true,
		Flags: func(env *cli.Env) *flag.FlagSet {
			set := env.NewFlagSet("flush")
			set.StringVar(&book, "book", "", "book id")
			set.BoolVar(&all, "all", false, "every book")
			set.BoolVar(&write, "write", false, "delete the listed files")
			return set
		},
		Run: func(env *cli.Env, args []string) (*cli.Result, error) {
			if len(args) != 0 {
				return nil, cli.Usage("usage.unexpected-argument", "flush takes flags only")
			}
			if all && book != "" {
				return nil, cli.Usage("usage.invalid-arguments", "use --book or --all, not both")
			}
			if !all && book == "" {
				return nil, cli.Usage("usage.missing-argument", "pass --book or --all").
					WithHint("pecia flush --book <id> --write")
			}
			var id model.BookID
			if !all {
				id = model.BookID(book)
				if !id.Valid() {
					return nil, cli.Usage("usage.invalid-value", "invalid book id %q", book)
				}
			}
			values, configErr := config.Load(env)
			if configErr != nil {
				return nil, configErr
			}
			service, serviceErr := New(values.TempRoot)
			if serviceErr != nil {
				return nil, cli.AsError(serviceErr)
			}
			entries, listErr := service.List(id, all)
			if listErr != nil {
				return nil, cli.AsError(listErr)
			}
			deleted := 0
			dryRun := env.Globals.DryRun || !write
			if !dryRun {
				count, deleteErr := service.Delete(entries)
				if deleteErr != nil {
					return nil, cli.AsError(deleteErr)
				}
				deleted = count
			}
			payload := payload{DryRun: dryRun, Deleted: deleted, Files: make([]filePayload, 0, len(entries))}
			for _, entry := range entries {
				payload.Files = append(payload.Files, filePayload{
					Path: entry.Path, Book: entry.Book, Size: entry.Size,
				})
			}
			return &cli.Result{Data: payload}, nil
		},
	})
}
