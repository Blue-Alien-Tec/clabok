package cli

import (
	"errors"
	"flag"
	"io"
	"strings"
)

// Globals are the flags accepted by every command.
type Globals struct {
	Project   string
	Config    string
	StoreRoot string
	TempRoot  string
	DocsRoot  string
	Text      bool
	DryRun    bool
	Help      bool
}

// Env is the execution context handed to a command.
type Env struct {
	Out     io.Writer
	Err     io.Writer
	Globals Globals
}

// NewFlagSet returns a flag set that accepts the global flags. A command adds its own flags to it.
func (e *Env) NewFlagSet(name string) *flag.FlagSet {
	set := flag.NewFlagSet(name, flag.ContinueOnError)
	set.SetOutput(io.Discard)
	set.StringVar(&e.Globals.Project, "project", e.Globals.Project, "project directory")
	set.StringVar(&e.Globals.Config, "config", e.Globals.Config, "configuration file")
	set.StringVar(&e.Globals.StoreRoot, "store-root", e.Globals.StoreRoot, "working store root")
	set.StringVar(&e.Globals.TempRoot, "temp-root", e.Globals.TempRoot, "temp root")
	set.StringVar(&e.Globals.DocsRoot, "docs-root", e.Globals.DocsRoot, "documentation root")
	set.BoolVar(&e.Globals.Text, "text", e.Globals.Text, "human-readable output")
	set.BoolVar(&e.Globals.DryRun, "dry-run", e.Globals.DryRun, "report the change without writing")
	set.BoolVar(&e.Globals.Help, "help", e.Globals.Help, "show help for the command")
	return set
}

// Parse parses the leading global flags and stops at the first positional (the command name).
func (e *Env) Parse(set *flag.FlagSet, args []string) (bool, *Error) {
	err := set.Parse(args)
	if err == nil {
		return false, nil
	}
	if errors.Is(err, flag.ErrHelp) {
		return true, nil
	}
	return false, flagError(err)
}

// ParseCommand parses a command line whose flags may appear before, between or after the
// positional arguments. It returns the positionals in order.
func (e *Env) ParseCommand(set *flag.FlagSet, args []string) ([]string, bool, *Error) {
	var positionals []string
	rest := args
	for {
		if err := set.Parse(rest); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return positionals, true, nil
			}
			return nil, false, flagError(err)
		}
		remaining := set.Args()
		if len(remaining) == 0 {
			break
		}
		positionals = append(positionals, remaining[0])
		rest = remaining[1:]
	}
	return positionals, false, nil
}

func flagError(err error) *Error {
	message := err.Error()
	if strings.Contains(message, "flag provided but not defined") {
		return Usage("usage.unknown-flag", "%s", message).
			WithHint("run with --help to list the accepted flags")
	}
	return Usage("usage.invalid-value", "%s", message)
}
