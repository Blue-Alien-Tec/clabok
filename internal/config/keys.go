package config

import (
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
)

const asciiuiPrefix = "asciiui.types."

type binding struct {
	name         string
	defaultValue any
	get          func(*file) (any, bool)
	set          func(*file, string) *cli.Error
	unset        func(*file)
}

var keyBindings = map[string]binding{
	"store.root": {
		name:         "store.root",
		defaultValue: builtin.StoreRoot,
		get: func(document *file) (any, bool) {
			if document.Store != nil && document.Store.Root != nil {
				return *document.Store.Root, true
			}
			return nil, false
		},
		set: func(document *file, raw string) *cli.Error {
			return setRootValue(&document.Store, "store.root", raw)
		},
		unset: func(document *file) {
			if document.Store != nil {
				document.Store.Root = nil
			}
		},
	},
	"temp.root": {
		name:         "temp.root",
		defaultValue: builtin.TempRoot,
		get: func(document *file) (any, bool) {
			if document.Temp != nil && document.Temp.Root != nil {
				return *document.Temp.Root, true
			}
			return nil, false
		},
		set: func(document *file, raw string) *cli.Error {
			return setRootValue(&document.Temp, "temp.root", raw)
		},
		unset: func(document *file) {
			if document.Temp != nil {
				document.Temp.Root = nil
			}
		},
	},
	"docs.root": {
		name:         "docs.root",
		defaultValue: builtin.DocsRoot,
		get: func(document *file) (any, bool) {
			if document.Docs != nil && document.Docs.Root != nil {
				return *document.Docs.Root, true
			}
			return nil, false
		},
		set: func(document *file, raw string) *cli.Error {
			return setRootValue(&document.Docs, "docs.root", raw)
		},
		unset: func(document *file) {
			if document.Docs != nil {
				document.Docs.Root = nil
			}
		},
	},
	"rollback.undoLimit": {
		name:         "rollback.undoLimit",
		defaultValue: builtin.UndoLimit,
		get: func(document *file) (any, bool) {
			if value, ok := document.undoLimit(); ok {
				return value, true
			}
			return nil, false
		},
		set: func(document *file, raw string) *cli.Error {
			return setLimit(&document.Rollback, true, "rollback.undoLimit", raw)
		},
		unset: func(document *file) {
			if document.Rollback != nil {
				document.Rollback.UndoLimit = nil
			}
		},
	},
	"rollback.bookPointLimit": {
		name:         "rollback.bookPointLimit",
		defaultValue: builtin.BookPointLimit,
		get: func(document *file) (any, bool) {
			if value, ok := document.bookPointLimit(); ok {
				return value, true
			}
			return nil, false
		},
		set: func(document *file, raw string) *cli.Error {
			return setLimit(&document.Rollback, false, "rollback.bookPointLimit", raw)
		},
		unset: func(document *file) {
			if document.Rollback != nil {
				document.Rollback.BookPointLimit = nil
			}
		},
	},
	"grammar.endpoint": {
		name:         "grammar.endpoint",
		defaultValue: builtin.GrammarEndpoint,
		get: func(document *file) (any, bool) {
			if document.Grammar != nil && document.Grammar.Endpoint != nil {
				return *document.Grammar.Endpoint, true
			}
			return nil, false
		},
		set: func(document *file, raw string) *cli.Error {
			return setEndpoint(&document.Grammar, raw)
		},
		unset: func(document *file) {
			if document.Grammar != nil {
				document.Grammar.Endpoint = nil
			}
		},
	},
	"grammar.policy": {
		name:         "grammar.policy",
		defaultValue: builtin.GrammarPolicy,
		get: func(document *file) (any, bool) {
			if document.Grammar != nil && document.Grammar.Policy != nil {
				return *document.Grammar.Policy, true
			}
			return nil, false
		},
		set: func(document *file, raw string) *cli.Error {
			return setPolicy(&document.Grammar, raw)
		},
		unset: func(document *file) {
			if document.Grammar != nil {
				document.Grammar.Policy = nil
			}
		},
	},
	"grammar.timeout": {
		name:         "grammar.timeout",
		defaultValue: builtin.GrammarTimeout,
		get: func(document *file) (any, bool) {
			if document.Grammar != nil && document.Grammar.Timeout != nil {
				return *document.Grammar.Timeout, true
			}
			return nil, false
		},
		set: func(document *file, raw string) *cli.Error {
			return setTimeout(&document.Grammar, raw)
		},
		unset: func(document *file) {
			if document.Grammar != nil {
				document.Grammar.Timeout = nil
			}
		},
	},
	"concurrency.mode": {
		name:         "concurrency.mode",
		defaultValue: builtin.ConcurrencyMode,
		get: func(document *file) (any, bool) {
			if document.Concurrency != nil && document.Concurrency.Mode != nil {
				return *document.Concurrency.Mode, true
			}
			return nil, false
		},
		set: func(document *file, raw string) *cli.Error {
			return setMode(document, raw)
		},
		unset: func(document *file) {
			if document.Concurrency != nil {
				document.Concurrency.Mode = nil
			}
		},
	},
	"concurrency.daemon.socket": {
		name:         "concurrency.daemon.socket",
		defaultValue: builtin.DaemonSocket,
		get: func(document *file) (any, bool) {
			if document.Concurrency != nil && document.Concurrency.Daemon != nil &&
				document.Concurrency.Daemon.Socket != nil {
				return *document.Concurrency.Daemon.Socket, true
			}
			return nil, false
		},
		set: func(document *file, raw string) *cli.Error {
			return setSocket(document, raw)
		},
		unset: func(document *file) {
			if document.Concurrency != nil && document.Concurrency.Daemon != nil {
				document.Concurrency.Daemon.Socket = nil
			}
		},
	},
}

func asciiuiName(key string) (string, bool) {
	if !strings.HasPrefix(key, asciiuiPrefix) {
		return "", false
	}
	name := strings.TrimPrefix(key, asciiuiPrefix)
	return name, name != ""
}

// knownKeys lists the addressable keys for error details.
func knownKeys() []string {
	names := make([]string, 0, len(keyBindings)+1)
	for name := range keyBindings {
		names = append(names, name)
	}
	names = append(names, asciiuiPrefix+"<name>")
	sort.Strings(names)
	return names
}

func unknownKeyError(key string) *cli.Error {
	return cli.Usage("usage.unknown-key", "unknown configuration key %q", key).
		WithHint("run `pecia config show` for the effective values").
		WithDetails(map[string]any{"known": knownKeys()})
}

func ensureRoot(section **rootSection) *rootSection {
	if *section == nil {
		*section = &rootSection{}
	}
	return *section
}

func setRootValue(section **rootSection, key, raw string) *cli.Error {
	value := strings.TrimSpace(unquote(raw))
	if issue := checkRoot(key, value); issue != nil {
		return cli.Usage("usage.invalid-value", "%s: %s", key, issue.Message).
			WithDetails(map[string]any{"key": key})
	}
	ensureRoot(section).Root = &value
	return nil
}

func setLimit(section **rollbackSection, undo bool, key, raw string) *cli.Error {
	value, err := strconv.Atoi(strings.TrimSpace(unquote(raw)))
	if err != nil || value < 0 {
		return cli.Usage("usage.invalid-value", "%s: expected a non-negative integer, got %q", key, raw).
			WithDetails(map[string]any{"key": key})
	}
	if *section == nil {
		*section = &rollbackSection{}
	}
	if undo {
		(*section).UndoLimit = &value
	} else {
		(*section).BookPointLimit = &value
	}
	return nil
}

func setEndpoint(section **grammarSection, raw string) *cli.Error {
	value := strings.TrimSpace(unquote(raw))
	if value != "" {
		parsed, err := url.Parse(value)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return cli.Usage("usage.invalid-value",
				"grammar.endpoint: expected an http(s) URL or an empty value, got %q", raw).
				WithDetails(map[string]any{"key": "grammar.endpoint"})
		}
	}
	if *section == nil {
		*section = &grammarSection{}
	}
	(*section).Endpoint = &value
	return nil
}

func setPolicy(section **grammarSection, raw string) *cli.Error {
	value := strings.TrimSpace(unquote(raw))
	switch value {
	case "block", "warn":
	default:
		return cli.Usage("usage.invalid-value",
			"grammar.policy: expected block or warn, got %q", raw).
			WithDetails(map[string]any{"key": "grammar.policy", "expected": []string{"block", "warn"}})
	}
	if *section == nil {
		*section = &grammarSection{}
	}
	(*section).Policy = &value
	return nil
}

func setTimeout(section **grammarSection, raw string) *cli.Error {
	value := strings.TrimSpace(unquote(raw))
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return cli.Usage("usage.invalid-value",
			"grammar.timeout: expected a positive duration such as 5s, got %q", raw).
			WithDetails(map[string]any{"key": "grammar.timeout"})
	}
	if *section == nil {
		*section = &grammarSection{}
	}
	(*section).Timeout = &value
	return nil
}

func setMode(document *file, raw string) *cli.Error {
	value := strings.TrimSpace(unquote(raw))
	if value != "file" && value != "daemon" {
		return cli.Usage("usage.invalid-value",
			"concurrency.mode: expected file or daemon, got %q", raw).
			WithDetails(map[string]any{"key": "concurrency.mode", "expected": []string{"file", "daemon"}})
	}
	if document.Concurrency == nil {
		document.Concurrency = &concurrencySection{}
	}
	document.Concurrency.Mode = &value
	return nil
}

func setSocket(document *file, raw string) *cli.Error {
	value := strings.TrimSpace(unquote(raw))
	if value == "" {
		if document.Concurrency != nil && document.Concurrency.Daemon != nil {
			document.Concurrency.Daemon.Socket = nil
		}
		return nil
	}
	if document.Concurrency == nil {
		document.Concurrency = &concurrencySection{}
	}
	if document.Concurrency.Daemon == nil {
		document.Concurrency.Daemon = &daemonSection{}
	}
	document.Concurrency.Daemon.Socket = &value
	return nil
}

// lookupValue returns the stored or default value without path resolution and without command
// flags. It is what a write compares before and after.
func lookupValue(document *file, key string) (any, string, *cli.Error) {
	if name, ok := asciiuiName(key); ok {
		value, exists := document.asciiuiTypes()[name]
		if exists {
			return value, "file", nil
		}
		return "", "default", nil
	}
	spec, ok := keyBindings[key]
	if !ok {
		return nil, "", unknownKeyError(key)
	}
	if value, set := spec.get(document); set {
		return value, "file", nil
	}
	return spec.defaultValue, "default", nil
}

// getValue returns the effective value: resolved paths and command flags applied.
func getValue(document *file, location Location, globals cli.Globals, key string) (any, string, *cli.Error) {
	if name, ok := asciiuiName(key); ok {
		value, exists := document.asciiuiTypes()[name]
		if exists {
			return value, "file", nil
		}
		return builtinTypeName(name), "default", nil
	}
	raw, source, err := lookupValue(document, key)
	if err != nil {
		return nil, "", err
	}
	values := resolve(document, location, globals)
	switch key {
	case "store.root":
		return values.StoreRoot, source, nil
	case "temp.root":
		return values.TempRoot, source, nil
	case "docs.root":
		return values.DocsRoot, source, nil
	}
	return raw, source, nil
}

// builtinTypeName is the generic default for an asciiui type: the name itself.
func builtinTypeName(name string) string {
	return name
}

func setValue(document *file, key, raw string) *cli.Error {
	if name, ok := asciiuiName(key); ok {
		value := strings.TrimSpace(unquote(raw))
		if value == "" {
			if document.Asciiui != nil {
				delete(document.Asciiui.Types, name)
			}
			return nil
		}
		if document.Asciiui == nil {
			document.Asciiui = &asciiuiSection{}
		}
		if document.Asciiui.Types == nil {
			document.Asciiui.Types = map[string]string{}
		}
		document.Asciiui.Types[name] = value
		return nil
	}
	spec, ok := keyBindings[key]
	if !ok {
		return unknownKeyError(key)
	}
	return spec.set(document, raw)
}

func unsetValue(document *file, key string) *cli.Error {
	if name, ok := asciiuiName(key); ok {
		if document.Asciiui != nil {
			delete(document.Asciiui.Types, name)
		}
		return nil
	}
	spec, ok := keyBindings[key]
	if !ok {
		return unknownKeyError(key)
	}
	spec.unset(document)
	return nil
}
