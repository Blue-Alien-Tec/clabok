// Package config loads the project configuration (`clabok.json`), resolves the effective values and
// implements the `config` command group.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/jsonstrict"
)

const (
	// Version is the supported configuration schema version.
	Version = 1
	// FileName is the project configuration file name.
	FileName = "clabok.json"
)

type rootSection struct {
	Root *string `json:"root,omitempty"`
}

type rollbackSection struct {
	UndoLimit      *int `json:"undoLimit,omitempty"`
	BookPointLimit *int `json:"bookPointLimit,omitempty"`
}

type grammarSection struct {
	Endpoint *string `json:"endpoint,omitempty"`
	Policy   *string `json:"policy,omitempty"`
	Timeout  *string `json:"timeout,omitempty"`
	Language *string `json:"language,omitempty"`
}

type daemonSection struct {
	Socket *string `json:"socket,omitempty"`
}

type concurrencySection struct {
	Mode   *string        `json:"mode,omitempty"`
	Daemon *daemonSection `json:"daemon,omitempty"`
}

type asciiuiSection struct {
	Types map[string]string `json:"types,omitempty"`
}

// file is the on-disk document. Every leaf is a pointer so that "absent" and "explicitly zero"
// stay distinguishable; `unset` restores the default instead of writing a zero.
type file struct {
	Version     *int                `json:"version,omitempty"`
	Store       *rootSection        `json:"store,omitempty"`
	Temp        *rootSection        `json:"temp,omitempty"`
	Docs        *rootSection        `json:"docs,omitempty"`
	Rollback    *rollbackSection    `json:"rollback,omitempty"`
	Grammar     *grammarSection     `json:"grammar,omitempty"`
	Concurrency *concurrencySection `json:"concurrency,omitempty"`
	Asciiui     *asciiuiSection     `json:"asciiui,omitempty"`
}

type builtinDefaults struct {
	StoreRoot       string
	TempRoot        string
	DocsRoot        string
	UndoLimit       int
	BookPointLimit  int
	GrammarEndpoint string
	GrammarPolicy   string
	GrammarTimeout  string
	GrammarLanguage string
	ConcurrencyMode string
	DaemonSocket    string
}

var builtin = builtinDefaults{
	StoreRoot:       "_CLABOK",
	TempRoot:        ".temp",
	DocsRoot:        "Docs",
	UndoLimit:       10,
	BookPointLimit:  3,
	GrammarEndpoint: "",
	GrammarPolicy:   "block",
	GrammarTimeout:  "5s",
	GrammarLanguage: "en-US",
	ConcurrencyMode: "file",
	DaemonSocket:    "",
}

func (f *file) storeRoot() string {
	if f.Store != nil && f.Store.Root != nil {
		return *f.Store.Root
	}
	return ""
}

func (f *file) tempRoot() string {
	if f.Temp != nil && f.Temp.Root != nil {
		return *f.Temp.Root
	}
	return ""
}

func (f *file) docsRoot() string {
	if f.Docs != nil && f.Docs.Root != nil {
		return *f.Docs.Root
	}
	return ""
}

func (f *file) undoLimit() (int, bool) {
	if f.Rollback != nil && f.Rollback.UndoLimit != nil {
		return *f.Rollback.UndoLimit, true
	}
	return 0, false
}

func (f *file) bookPointLimit() (int, bool) {
	if f.Rollback != nil && f.Rollback.BookPointLimit != nil {
		return *f.Rollback.BookPointLimit, true
	}
	return 0, false
}

func (f *file) grammarEndpoint() string {
	if f.Grammar != nil && f.Grammar.Endpoint != nil {
		return *f.Grammar.Endpoint
	}
	return ""
}

func (f *file) grammarPolicy() string {
	if f.Grammar != nil && f.Grammar.Policy != nil {
		return *f.Grammar.Policy
	}
	return ""
}

func (f *file) grammarTimeout() string {
	if f.Grammar != nil && f.Grammar.Timeout != nil {
		return *f.Grammar.Timeout
	}
	return ""
}

func (f *file) grammarLanguage() string {
	if f.Grammar != nil && f.Grammar.Language != nil {
		return *f.Grammar.Language
	}
	return ""
}

func (f *file) concurrencyMode() string {
	if f.Concurrency != nil && f.Concurrency.Mode != nil {
		return *f.Concurrency.Mode
	}
	return ""
}

func (f *file) daemonSocket() string {
	if f.Concurrency != nil && f.Concurrency.Daemon != nil && f.Concurrency.Daemon.Socket != nil {
		return *f.Concurrency.Daemon.Socket
	}
	return ""
}

func (f *file) asciiuiTypes() map[string]string {
	if f.Asciiui == nil {
		return nil
	}
	return f.Asciiui.Types
}

func (f *file) prune() {
	if f.Store != nil && f.Store.Root == nil {
		f.Store = nil
	}
	if f.Temp != nil && f.Temp.Root == nil {
		f.Temp = nil
	}
	if f.Docs != nil && f.Docs.Root == nil {
		f.Docs = nil
	}
	if f.Rollback != nil && f.Rollback.UndoLimit == nil && f.Rollback.BookPointLimit == nil {
		f.Rollback = nil
	}
	if f.Grammar != nil && f.Grammar.Endpoint == nil && f.Grammar.Policy == nil &&
		f.Grammar.Timeout == nil && f.Grammar.Language == nil {
		f.Grammar = nil
	}
	if f.Concurrency != nil {
		if f.Concurrency.Daemon != nil && f.Concurrency.Daemon.Socket == nil {
			f.Concurrency.Daemon = nil
		}
		if f.Concurrency.Mode == nil && f.Concurrency.Daemon == nil {
			f.Concurrency = nil
		}
	}
	if f.Asciiui != nil && len(f.Asciiui.Types) == 0 {
		f.Asciiui = nil
	}
}

func decode(data []byte) (*file, *cli.Error) {
	var document file
	err := jsonstrict.Decode(data, &document, jsonstrict.Options{DisallowUnknownFields: true})
	if err != nil {
		var strict *jsonstrict.Error
		if errors.As(err, &strict) {
			switch strict.Kind {
			case jsonstrict.KindUnknownField:
				return nil, cli.Fail("config.unknown-key", "%s", strict.Message).
					WithHint("remove the field or check the configuration documentation")
			case jsonstrict.KindDuplicateKey:
				return nil, cli.Fail("config.duplicate-key", "%s", strict.Message).
					WithHint("keep exactly one occurrence of the key")
			}
		}
		return nil, cli.Fail("config.invalid-json", "%v", err).
			WithHint("the configuration must be one JSON object")
	}
	if document.Version == nil {
		return nil, cli.Fail("config.version-missing", "the configuration has no version field").
			WithHint(fmt.Sprintf("add \"version\": %d", Version))
	}
	if *document.Version != Version {
		if *document.Version > Version {
			return nil, cli.Fail("config.version-unsupported",
				"configuration version %d is newer than pecia supports (%d)",
				*document.Version, Version).
				WithHint("upgrade pecia or lower the version field")
		}
		return nil, cli.Fail("config.version-unsupported",
			"unsupported configuration version %d (expected %d)",
			*document.Version, Version)
	}
	return &document, nil
}

func encode(document *file) ([]byte, error) {
	version := Version
	document.Version = &version
	document.prune()
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func unquote(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) >= 2 && trimmed[0] == '"' && trimmed[len(trimmed)-1] == '"' {
		var text string
		if err := json.Unmarshal([]byte(trimmed), &text); err == nil {
			return text
		}
	}
	return raw
}
