package cli

import (
	"fmt"
	"io"
	"runtime/debug"

	"github.com/Blue-Alien-Tec/clabok/internal/buildinfo"
)

// Run executes one command line and returns the process exit code.
//
// The function is the single output boundary: it resolves the command, parses its flags, enforces
// the dry-run contract, emits the envelope or the text form, and maps the outcome to an exit code.
func Run(registry *Registry, args []string, stdout, stderr io.Writer) int {
	env := &Env{Out: stdout, Err: stderr}

	pre := env.NewFlagSet(buildinfo.CLI)
	help, parseErr := env.Parse(pre, args)
	if parseErr != nil {
		emitError(env, buildinfo.CLI, parseErr)
		return ExitCode(parseErr.Kind)
	}
	if help || env.Globals.Help {
		return emitResult(env, "help", helpListResult(registry))
	}

	rest := pre.Args()
	if len(rest) == 0 {
		err := Usage("usage.missing-command", "no command given").
			WithHint("run `pecia help` for the command list").
			WithDetails(map[string]any{"commands": commandNames(registry)})
		emitError(env, buildinfo.CLI, err)
		return ExitCode(err.Kind)
	}

	command, consumed, attempted := registry.Resolve(rest)
	if command == nil {
		return emitResolveError(env, registry, attempted)
	}

	flags := env.NewFlagSet(command.Name)
	if command.Flags != nil {
		flags = command.Flags(env)
	}
	positionals, help, parseErr := env.ParseCommand(flags, rest[consumed:])
	if parseErr != nil {
		emitError(env, command.Name, parseErr)
		return ExitCode(parseErr.Kind)
	}
	if help || env.Globals.Help {
		result, _ := commandHelpResult(env, registry, actionTopicName(command, positionals))
		return emitResult(env, command.Name, result)
	}

	result, err := call(command, env, positionals)
	if err != nil {
		outcome := AsError(err)
		emitError(env, command.Name, outcome)
		return ExitCode(outcome.Kind)
	}
	if result == nil {
		outcome := Fail("internal.empty-result", "command %q returned no result", command.Name)
		emitError(env, command.Name, outcome)
		return ExitFailure
	}
	if command.Mutating && env.Globals.DryRun {
		if _, ok := result.Data.(DryRunResult); !ok {
			outcome := Fail("internal.dry-run-missing",
				"command %q did not report a dry-run receipt", command.Name)
			emitError(env, command.Name, outcome)
			return ExitFailure
		}
	}
	return emitResult(env, command.Name, result)
}

func emitResolveError(env *Env, registry *Registry, attempted string) int {
	if registry.HasPrefix(attempted) {
		err := Usage("usage.missing-subcommand", "%q needs a subcommand", attempted).
			WithHint("run `pecia help` for the command list").
			WithDetails(map[string]any{"commands": commandNames(registry)})
		emitError(env, attempted, err)
		return ExitUsage
	}
	err := Usage("usage.unknown-command", "unknown command %q", attempted).
		WithHint("run `pecia help` for the command list")
	if suggestion := registry.Suggest(attempted); suggestion != "" {
		err = err.WithDetails(map[string]any{"suggestion": suggestion})
	}
	emitError(env, attempted, err)
	return ExitUsage
}

func call(command *Command, env *Env, args []string) (result *Result, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			fmt.Fprintf(env.Err, "panic: %v\n%s\n", recovered, debug.Stack())
			result = nil
			err = Fail("internal.panic", "internal error: %v", recovered)
		}
	}()
	return command.Run(env, args)
}

func emitResult(env *Env, command string, result *Result) int {
	code := ExitOK
	if env.Globals.Text {
		fmt.Fprint(env.Out, ensureTrailingNewline(textOf(result)))
	} else if len(result.Raw) > 0 {
		if _, err := env.Out.Write(result.Raw); err != nil {
			fmt.Fprintf(env.Err, "error: %v\n", err)
			return ExitFailure
		}
	} else {
		payload := envelope{
			Schema:  SchemaVersion,
			OK:      true,
			Command: command,
			Data:    result.Data,
		}
		if err := writeJSON(env.Out, payload); err != nil {
			fmt.Fprintf(env.Err, "error: %v\n", err)
			return ExitFailure
		}
	}
	if result.Notice != nil {
		emitError(env, command, result.Notice)
	}
	if result.Exit != 0 {
		code = result.Exit
	}
	return code
}

func emitError(env *Env, command string, outcome *Error) {
	if env.Globals.Text {
		fmt.Fprintf(env.Err, "error: %s\n", outcome.Message)
		if outcome.Hint != "" {
			fmt.Fprintf(env.Err, "hint: %s\n", outcome.Hint)
		}
		if texter, ok := outcome.Details.(Texter); ok {
			fmt.Fprint(env.Err, ensureTrailingNewline(texter.Text()))
		}
		return
	}
	payload := envelope{
		Schema:  SchemaVersion,
		OK:      false,
		Command: command,
		Error: &errorObject{
			Code:    outcome.Code,
			Message: outcome.Message,
			Hint:    outcome.Hint,
			Details: outcome.Details,
		},
	}
	_ = writeJSON(env.Err, payload)
}

func commandNames(registry *Registry) []string {
	commands := registry.List()
	names := make([]string, 0, len(commands))
	for _, command := range commands {
		names = append(names, command.Name)
	}
	return names
}
