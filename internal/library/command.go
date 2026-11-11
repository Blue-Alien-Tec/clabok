package library

import (
	"flag"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/render"
)

// Register adds the book lifecycle and library commands.
func Register(registry *cli.Registry) {
	registry.Add(newBookCommand())
	registry.Add(moveBookCommand())
	registry.Add(renameBookCommand())
	registry.Add(unregisterBookCommand())
	registry.Add(purgeBookCommand())
	registry.Add(defaultsCommand())
	registry.Add(syncCommand())
}

func newBookCommand() *cli.Command {
	var title, id, outputPath, outputMode string
	var numbering, toc, allowOutside, libraryBook, force bool
	var indexDepth int
	var flags *flag.FlagSet
	return &cli.Command{
		Name:     "book new",
		Summary:  "create, register and render a book",
		Usage:    "pecia book new --title <title> [--id <id>] [--path <dir>] [--allow-outside] [--library] [--force]",
		Mutating: true,
		Flags: func(env *cli.Env) *flag.FlagSet {
			set := env.NewFlagSet("book new")
			set.StringVar(&title, "title", "", "book title")
			set.StringVar(&id, "id", "", "book id (default: slug of the title)")
			set.StringVar(&outputPath, "path", "", "output directory, relative to the project")
			set.StringVar(&outputMode, "output", "", "output mode: monolith, chapter or part")
			set.BoolVar(&numbering, "numbering", false, "enable numbering")
			set.BoolVar(&toc, "toc", false, "enable the table of contents")
			set.IntVar(&indexDepth, "index-depth", -1, "index depth")
			set.BoolVar(&allowOutside, "allow-outside", false, "allow a path outside the docs root")
			set.BoolVar(&libraryBook, "library", false, "create the library book at the project root")
			set.BoolVar(&force, "force", false, "render into a non-empty directory and back up foreign files")
			flags = set
			return set
		},
		Run: func(env *cli.Env, args []string) (*cli.Result, error) {
			if len(args) != 0 {
				return nil, cli.Usage("usage.unexpected-argument", "book new takes flags only")
			}
			options := NewBookOptions{
				Title: title, ID: id, Path: outputPath, AllowOutside: allowOutside,
				Library: libraryBook, Force: force, Output: outputMode,
			}
			if wasSet(flags, "numbering") {
				value := numbering
				options.Numbering = &value
			}
			if wasSet(flags, "toc") {
				value := toc
				options.TOC = &value
			}
			if wasSet(flags, "index-depth") {
				value := indexDepth
				options.IndexDepth = &value
			}
			service, serviceErr := FromEnv(env)
			if serviceErr != nil {
				return nil, serviceErr
			}
			result, createErr := service.NewBook(options, env.Globals.DryRun)
			if createErr != nil {
				return nil, createErr
			}
			return &cli.Result{Data: result}, nil
		},
	}
}

func moveBookCommand() *cli.Command {
	var to string
	var allowOutside bool
	return &cli.Command{
		Name:     "book move",
		Summary:  "change a book's registry output path",
		Usage:    "pecia book move <book-id> --to <dir> [--allow-outside]",
		Mutating: true,
		Flags: func(env *cli.Env) *flag.FlagSet {
			set := env.NewFlagSet("book move")
			set.StringVar(&to, "to", "", "new output directory, relative to the project")
			set.BoolVar(&allowOutside, "allow-outside", false, "allow a path outside the docs root")
			return set
		},
		Run: func(env *cli.Env, args []string) (*cli.Result, error) {
			id, idErr := bookIDArg(args, "book move")
			if idErr != nil {
				return nil, idErr
			}
			service, serviceErr := FromEnv(env)
			if serviceErr != nil {
				return nil, serviceErr
			}
			result, moveErr := service.Move(MoveOptions{
				ID: id, Path: to, AllowOutside: allowOutside,
			}, env.Globals.DryRun)
			if moveErr != nil {
				return nil, moveErr
			}
			return &cli.Result{Data: result}, nil
		},
	}
}

func renameBookCommand() *cli.Command {
	var title string
	return &cli.Command{
		Name:     "book rename",
		Summary:  "change a book title",
		Usage:    "pecia book rename <book-id> --title <title>",
		Mutating: true,
		Flags: func(env *cli.Env) *flag.FlagSet {
			set := env.NewFlagSet("book rename")
			set.StringVar(&title, "title", "", "new title")
			return set
		},
		Run: func(env *cli.Env, args []string) (*cli.Result, error) {
			id, idErr := bookIDArg(args, "book rename")
			if idErr != nil {
				return nil, idErr
			}
			service, serviceErr := FromEnv(env)
			if serviceErr != nil {
				return nil, serviceErr
			}
			result, renameErr := service.Rename(RenameOptions{ID: id, Title: title}, env.Globals.DryRun)
			if renameErr != nil {
				return nil, renameErr
			}
			return &cli.Result{Data: result}, nil
		},
	}
}

func unregisterBookCommand() *cli.Command {
	var purge bool
	return &cli.Command{
		Name:     "book unregister",
		Summary:  "remove a book from the registry; --purge also deletes its working data",
		Usage:    "pecia book unregister <book-id> [--purge]",
		Mutating: true,
		Flags: func(env *cli.Env) *flag.FlagSet {
			set := env.NewFlagSet("book unregister")
			set.BoolVar(&purge, "purge", false,
				"also delete the working data (store, rollback pool, temp inputs, lock files)")
			return set
		},
		Run: func(env *cli.Env, args []string) (*cli.Result, error) {
			id, idErr := bookIDArg(args, "book unregister")
			if idErr != nil {
				return nil, idErr
			}
			service, serviceErr := FromEnv(env)
			if serviceErr != nil {
				return nil, serviceErr
			}
			result, unregisterErr := service.Unregister(id, purge, env.Globals.DryRun)
			if unregisterErr != nil {
				return nil, unregisterErr
			}
			return &cli.Result{Data: result}, nil
		},
	}
}

func purgeBookCommand() *cli.Command {
	return &cli.Command{
		Name:     "book purge",
		Summary:  "delete the working data of an unregistered book; the rendered output stays",
		Usage:    "pecia book purge <book-id>",
		Mutating: true,
		Run: func(env *cli.Env, args []string) (*cli.Result, error) {
			id, idErr := bookIDArg(args, "book purge")
			if idErr != nil {
				return nil, idErr
			}
			service, serviceErr := FromEnv(env)
			if serviceErr != nil {
				return nil, serviceErr
			}
			result, purgeErr := service.Purge(id, env.Globals.DryRun)
			if purgeErr != nil {
				return nil, purgeErr
			}
			return &cli.Result{Data: result}, nil
		},
	}
}

func defaultsCommand() *cli.Command {
	actions := []cli.Action{
		{Name: "show", Summary: "show the library defaults", Reads: []string{}},
		{Name: "set", Summary: "set one default as a key and a value", Reads: []string{}},
		{Name: "unset", Summary: "reset every default", Reads: []string{}},
	}
	return &cli.Command{
		Name:     "library defaults",
		Summary:  "show, change or reset the library defaults",
		Usage:    "pecia library defaults <show | set <key> <value> | unset>",
		Actions:  actions,
		Mutating: true,
		Run: func(env *cli.Env, args []string) (*cli.Result, error) {
			if len(args) == 0 {
				return nil, cli.Usage("usage.missing-argument",
					"library defaults needs %s", cli.ActionList(actions)).
					WithHint("pecia library defaults show")
			}
			service, serviceErr := FromEnv(env)
			if serviceErr != nil {
				return nil, serviceErr
			}
			switch args[0] {
			case "show":
				if len(args) != 1 {
					return nil, cli.Usage("usage.unexpected-argument", "library defaults show takes no value")
				}
				result, defaultsErr := service.Defaults()
				if defaultsErr != nil {
					return nil, defaultsErr
				}
				return &cli.Result{Data: result}, nil
			case "set":
				if len(args) != 3 {
					return nil, cli.Usage("usage.invalid-arguments",
						"library defaults set takes a key and a value").
						WithHint("pecia library defaults set numbering true")
				}
				result, setErr := service.SetDefault(args[1], args[2], env.Globals.DryRun)
				if setErr != nil {
					return nil, setErr
				}
				return &cli.Result{Data: result}, nil
			case "unset":
				if len(args) != 1 {
					return nil, cli.Usage("usage.unexpected-argument", "library defaults unset takes no value")
				}
				result, unsetErr := service.UnsetDefaults(env.Globals.DryRun)
				if unsetErr != nil {
					return nil, unsetErr
				}
				return &cli.Result{Data: result}, nil
			default:
				return nil, cli.Usage("usage.invalid-arguments",
					"unknown library defaults action %q; use %s", args[0], cli.ActionList(actions))
			}
		},
	}
}

func syncCommand() *cli.Command {
	var check, write, prune, force bool
	return &cli.Command{
		Name:     "library sync",
		Summary:  "render every registered book and the library index",
		Usage:    "pecia library sync [--check | --write] [--prune] [--force]",
		Mutating: true,
		Flags: func(env *cli.Env) *flag.FlagSet {
			set := env.NewFlagSet("library sync")
			set.BoolVar(&check, "check", false, "report drift and fail on it")
			set.BoolVar(&write, "write", false, "write changed files")
			set.BoolVar(&prune, "prune", false, "remove generated files the mode no longer produces")
			set.BoolVar(&force, "force", false, "back up and overwrite files without the generated marker")
			return set
		},
		Run: func(env *cli.Env, args []string) (*cli.Result, error) {
			if len(args) != 0 {
				return nil, cli.Usage("usage.unexpected-argument", "library sync takes flags only")
			}
			if check && write {
				return nil, cli.Usage("usage.invalid-arguments", "use --check or --write, not both")
			}
			result, syncErr := render.RunLibrarySync(env, check, write, prune, force)
			if syncErr != nil {
				return nil, syncErr
			}
			return result, nil
		},
	}
}

func wasSet(flags *flag.FlagSet, name string) bool {
	if flags == nil {
		return false
	}
	seen := false
	flags.Visit(func(current *flag.Flag) {
		if current.Name == name {
			seen = true
		}
	})
	return seen
}

func bookIDArg(args []string, command string) (model.BookID, *cli.Error) {
	if len(args) != 1 {
		return "", cli.Usage("usage.invalid-arguments", "%s takes exactly one book id", command).
			WithHint("pecia " + command + " <book-id>")
	}
	id := model.BookID(args[0])
	if !id.Valid() {
		return "", cli.Usage("usage.invalid-value", "invalid book id %q", args[0])
	}
	return id, nil
}
