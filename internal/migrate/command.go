package migrate

import (
	"errors"
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/config"
	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

// Text renders the human form of a report.
func (report Report) Text() string {
	var builder strings.Builder
	for _, book := range report.Books {
		steps := "up to date"
		if len(book.Manifest.Steps) > 0 {
			steps = strings.Join(book.Manifest.Steps, ", ")
		}
		fmt.Fprintf(&builder, "%s: manifest %d -> %d (%s), %s\n",
			book.ID, book.Manifest.From, book.Manifest.To, steps, book.State)
	}
	fmt.Fprintf(&builder, "registry: %d -> %d\n", report.Registry.From, report.Registry.To)
	fmt.Fprintf(&builder, "store: %d -> %d\n", report.Store.From, report.Store.To)
	fmt.Fprintf(&builder, "ledger: %d\n", report.Ledger.From)
	for _, warning := range report.Warnings {
		fmt.Fprintf(&builder, "warning: %s: %s\n", warning.Code, warning.Message)
	}
	return builder.String()
}

// Register adds the migrate command.
func Register(registry *cli.Registry) {
	var book string
	var all, write, strict bool
	var to, keep int
	actions := []cli.Action{
		{Name: "status", Summary: "report the manifest, store and registry state",
			Reads: []string{"book", "all"}},
		{Name: "plan", Summary: "plan the migration steps without writing",
			Reads: []string{"book", "all", "to", "strict"}},
		{Name: "apply", Summary: "apply the planned migration",
			Reads: []string{"book", "all", "to", "write", "strict"}},
		{Name: "cleanup", Summary: "prune migration history",
			Reads: []string{"book", "all", "keep", "write"}},
	}
	registry.Add(&cli.Command{
		Name:    "migrate",
		Summary: "report, plan and apply book and store schema migrations",
		Usage:   "pecia migrate <status|plan|apply|cleanup> [--book <id> | --all] [--to <version>] [--keep N] [--write] [--strict]",
		Actions: actions,
		Flags: func(env *cli.Env) *flag.FlagSet {
			set := env.NewFlagSet("migrate")
			set.StringVar(&book, "book", "", "book id")
			set.BoolVar(&all, "all", false, "every book plus the store metadata")
			set.IntVar(&to, "to", 0, "manifest target version")
			set.IntVar(&keep, "keep", 10, "cleanup: history entries to keep")
			set.BoolVar(&write, "write", false, "apply the plan")
			set.BoolVar(&strict, "strict", false, "fail on the first error")
			return set
		},
		Run: func(env *cli.Env, args []string) (*cli.Result, error) {
			action := "status"
			if len(args) == 1 {
				action = args[0]
			} else if len(args) > 1 {
				return nil, cli.Usage("usage.unexpected-argument", "migrate takes one action")
			}
			switch action {
			case "status", "plan", "apply", "cleanup":
			default:
				return nil, cli.Usage("usage.invalid-value",
					"unknown migrate action %q; use %s", action, cli.ActionList(actions))
			}
			if book != "" && !model.BookID(book).Valid() {
				return nil, cli.Usage("usage.invalid-value", "invalid book id %q", book)
			}
			if action == "cleanup" && (book != "" || all) {
				return nil, cli.Reject("migrate.scope-conflict", "migrate cleanup is store-wide")
			}
			life, layout, envErr := lifecycle.FromEnv(env)
			if envErr != nil {
				return nil, envErr
			}
			values, configErr := config.Load(env)
			if configErr != nil {
				return nil, configErr
			}
			registryValue, registryErr := store.LoadRegistry(layout.Registry())
			if registryErr != nil {
				registryValue = nil
			}
			service := New(life, layout, registryValue, values.BookPointLimit,
				filepath.Join(values.ProjectDir, config.FileName))
			var report Report
			var err error
			switch action {
			case "status":
				report, err = service.Status(model.BookID(book), all || book == "")
			case "plan":
				report, err = service.Apply(model.BookID(book), all, to, false, strict)
			case "apply":
				report, err = service.Apply(model.BookID(book), all, to, write && !env.Globals.DryRun, strict)
			case "cleanup":
				cleanup, cleanupErr := service.Cleanup(keep, write && !env.Globals.DryRun)
				if cleanupErr != nil {
					return nil, asCLI(cleanupErr)
				}
				return &cli.Result{Data: cleanup, Text: cleanup.Text()}, nil
			}
			if err != nil {
				return nil, asCLI(err)
			}
			return &cli.Result{Data: report, Text: report.Text()}, nil
		},
	})
}

// asCLI maps a migration error to the documented exit code.
func asCLI(err error) *cli.Error {
	var migrateErr *Error
	if !errors.As(err, &migrateErr) {
		return cli.AsError(err)
	}
	switch migrateErr.Code {
	case "migrate.scope-required", "migrate.scope-conflict", "migrate.unknown-book",
		"migrate.book-open", "migrate.downgrade", "migrate.unsupported-source",
		"migrate.unsupported-target", "migrate.registry-missing":
		return cli.Reject(migrateErr.Code, "%s", migrateErr.Message)
	case "migrate.strict-failed", "migrate.failed", "migrate.verify-failed",
		"migrate.read-failed", "migrate.write-failed":
		return cli.Fail(migrateErr.Code, "%s", migrateErr.Message)
	default:
		return cli.Usage(migrateErr.Code, "%s", migrateErr.Message)
	}
}
