package cli

import (
	"flag"
	"sort"
	"strings"
)

// Handler runs a command after its flags were parsed. args holds the positional arguments only.
type Handler func(*Env, []string) (*Result, error)

// Command is one entry of the command surface. Multi-word names such as "config show" are the
// canonical form; there is no separate subcommand tree.
type Command struct {
	Name     string
	Summary  string
	Usage    string
	Mutating bool
	// Flags builds the command flag set. It must start from Env.NewFlagSet so the global flags
	// stay accepted. A nil Flags means "global flags only".
	Flags func(*Env) *flag.FlagSet
	Run   Handler
}

// Registry holds the commands. Registration happens once during wiring.
type Registry struct {
	commands map[string]*Command
}

// NewRegistry returns a registry with the built-in commands.
func NewRegistry() *Registry {
	registry := &Registry{commands: make(map[string]*Command)}
	registerBuiltins(registry)
	return registry
}

// Add registers a command. A duplicate name or a missing handler is a programming error and
// panics during wiring, before any command runs.
func (r *Registry) Add(command *Command) {
	if command == nil || command.Name == "" || command.Run == nil {
		panic("cli: a command needs a name and a handler")
	}
	if _, exists := r.commands[command.Name]; exists {
		panic("cli: duplicate command " + command.Name)
	}
	if command.Usage == "" {
		command.Usage = "pecia " + command.Name
	}
	r.commands[command.Name] = command
}

// Get returns one command.
func (r *Registry) Get(name string) (*Command, bool) {
	command, ok := r.commands[name]
	return command, ok
}

// List returns the commands sorted by name.
func (r *Registry) List() []*Command {
	commands := make([]*Command, 0, len(r.commands))
	for _, command := range r.commands {
		commands = append(commands, command)
	}
	sort.Slice(commands, func(left, right int) bool {
		return commands[left].Name < commands[right].Name
	})
	return commands
}

// HasPrefix reports whether any command name extends name, which makes name a group.
func (r *Registry) HasPrefix(name string) bool {
	prefix := name + " "
	for commandName := range r.commands {
		if strings.HasPrefix(commandName, prefix) {
			return true
		}
	}
	return false
}

// Resolve finds the command for the leading positionals and reports how many were consumed.
func (r *Registry) Resolve(args []string) (*Command, int, string) {
	if len(args) == 0 {
		return nil, 0, ""
	}
	hasSecond := len(args) > 1 && !strings.HasPrefix(args[1], "-")
	if hasSecond {
		if command, ok := r.commands[args[0]+" "+args[1]]; ok {
			return command, 2, ""
		}
	}
	if command, ok := r.commands[args[0]]; ok {
		return command, 1, ""
	}
	if hasSecond {
		return nil, 0, args[0] + " " + args[1]
	}
	return nil, 0, args[0]
}

// Suggest returns the closest command name within a small edit distance, or "".
func (r *Registry) Suggest(name string) string {
	best := ""
	bestDistance := -1
	limit := len(name)/3 + 1
	for _, command := range r.commands {
		distance := editDistance(name, command.Name)
		if distance > limit {
			continue
		}
		if bestDistance < 0 || distance < bestDistance ||
			(distance == bestDistance && command.Name < best) {
			best = command.Name
			bestDistance = distance
		}
	}
	return best
}

// editDistance is the Levenshtein distance over bytes; command names are ASCII.
func editDistance(left, right string) int {
	if left == right {
		return 0
	}
	previous := make([]int, len(right)+1)
	current := make([]int, len(right)+1)
	for index := range previous {
		previous[index] = index
	}
	for i := 1; i <= len(left); i++ {
		current[0] = i
		for j := 1; j <= len(right); j++ {
			cost := 1
			if left[i-1] == right[j-1] {
				cost = 0
			}
			current[j] = min(previous[j]+1, current[j-1]+1, previous[j-1]+cost)
		}
		previous, current = current, previous
	}
	return previous[len(right)]
}
