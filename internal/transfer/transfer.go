// Package transfer implements the dump, restore, export and import commands. The dump form is a
// byte-exact container copy; the text form is a reviewable directory that remaps every ID.
package transfer

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/Blue-Alien-Tec/clabok/internal/buildinfo"
	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/config"
	"github.com/Blue-Alien-Tec/clabok/internal/idspace"
	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

const (
	exportFormat     = "clabok-text"
	exportVersion    = 1
	descriptorMember = "export.json"
	manifestMember   = lifecycle.ManifestMember
)

type codedError struct{ code, message string }

func (err *codedError) Error() string     { return err.code + ": " + err.message }
func (err *codedError) ErrorCode() string { return err.code }

func fail(code, format string, args ...any) error {
	return &codedError{code: code, message: fmt.Sprintf(format, args...)}
}

// Register adds the four transfer commands.
func Register(registry *cli.Registry) {
	registry.Add(dumpCommand())
	registry.Add(restoreCommand())
	registry.Add(exportCommand())
	registry.Add(importCommand())
}

type dumpPayload struct {
	DryRun bool   `json:"dryRun"`
	Action string `json:"action"`
	Book   string `json:"book"`
	File   string `json:"file"`
	Bytes  int    `json:"bytes"`
	SHA256 string `json:"sha256"`
	Match  *bool  `json:"match,omitempty"`
}

func (payload dumpPayload) DryRunResult() bool { return payload.DryRun }

func (payload dumpPayload) Text() string {
	if payload.Match != nil {
		state := "differs"
		if *payload.Match {
			state = "matches"
		}
		return fmt.Sprintf("%s: %s %s\n", payload.Book, payload.File, state)
	}
	return fmt.Sprintf("%s: dumped %d bytes to %s\n", payload.Book, payload.Bytes, payload.File)
}

func dumpCommand() *cli.Command {
	var book, out string
	var check bool
	return &cli.Command{
		Name:    "dump",
		Summary: "copy one book container byte for byte",
		Usage:   "pecia dump --book <id> --out <file> [--check]",
		Flags: func(env *cli.Env) *flag.FlagSet {
			set := env.NewFlagSet("dump")
			set.StringVar(&book, "book", "", "book id")
			set.StringVar(&out, "out", "", "artifact file")
			set.BoolVar(&check, "check", false, "compare the artifact with the current container")
			return set
		},
		Run: func(env *cli.Env, args []string) (*cli.Result, error) {
			if len(args) != 0 {
				return nil, cli.Usage("usage.invalid-arguments", "dump takes flags only")
			}
			if book == "" || out == "" {
				return nil, cli.Usage("usage.missing-argument", "--book and --out are required")
			}
			payload, err := runDump(env, book, out, check)
			if err != nil {
				return nil, cli.AsError(err)
			}
			return &cli.Result{Data: payload}, nil
		},
	}
}

type restorePayload struct {
	DryRun   bool   `json:"dryRun"`
	Action   string `json:"action"`
	Book     string `json:"book"`
	File     string `json:"file"`
	Version  int    `json:"version"`
	Replaced bool   `json:"replaced,omitempty"`
	Receipt  string `json:"receipt,omitempty"`
}

func (payload restorePayload) DryRunResult() bool { return payload.DryRun }

func (payload restorePayload) Text() string {
	return fmt.Sprintf("%s: restored manifest v%d from %s\n", payload.Book, payload.Version,
		payload.File)
}

func restoreCommand() *cli.Command {
	var file, as, target string
	var force, library bool
	return &cli.Command{
		Name:     "restore",
		Summary:  "restore one book container into the store",
		Usage:    "pecia restore --file <artifact> --as <book-id> [--path <dir>] [--library] [--force]",
		Mutating: true,
		Flags: func(env *cli.Env) *flag.FlagSet {
			set := env.NewFlagSet("restore")
			set.StringVar(&file, "file", "", "artifact file")
			set.StringVar(&as, "as", "", "target book id")
			set.StringVar(&target, "path", "", "output path of the restored book")
			set.BoolVar(&library, "library", false, "register the book as the library")
			set.BoolVar(&force, "force", false, "replace the registered book")
			return set
		},
		Run: func(env *cli.Env, args []string) (*cli.Result, error) {
			if len(args) != 0 {
				return nil, cli.Usage("usage.invalid-arguments", "restore takes flags only")
			}
			if file == "" || as == "" {
				return nil, cli.Usage("usage.missing-argument", "--file and --as are required")
			}
			payload, err := runRestore(env, file, as, target, library, force)
			if err != nil {
				return nil, cli.AsError(err)
			}
			return &cli.Result{Data: payload}, nil
		},
	}
}

type exportPayload struct {
	DryRun  bool     `json:"dryRun"`
	Action  string   `json:"action"`
	Book    string   `json:"book"`
	Dir     string   `json:"dir"`
	Members int      `json:"members"`
	Match   *bool    `json:"match,omitempty"`
	Missing []string `json:"missing,omitempty"`
	Extra   []string `json:"extra,omitempty"`
	Changed []string `json:"changed,omitempty"`
}

func (payload exportPayload) DryRunResult() bool { return payload.DryRun }

func (payload exportPayload) Text() string {
	if payload.Match != nil {
		state := "differs"
		if *payload.Match {
			state = "matches"
		}
		return fmt.Sprintf("%s: %s %s\n", payload.Book, payload.Dir, state)
	}
	return fmt.Sprintf("%s: exported %d member(s) to %s\n", payload.Book, payload.Members,
		payload.Dir)
}

func exportCommand() *cli.Command {
	var book, out string
	var check bool
	return &cli.Command{
		Name:    "export",
		Summary: "export one book as a reviewable text directory",
		Usage:   "pecia export --book <id> --out <dir> [--check]",
		Flags: func(env *cli.Env) *flag.FlagSet {
			set := env.NewFlagSet("export")
			set.StringVar(&book, "book", "", "book id")
			set.StringVar(&out, "out", "", "export directory")
			set.BoolVar(&check, "check", false, "compare the directory with the current book")
			return set
		},
		Run: func(env *cli.Env, args []string) (*cli.Result, error) {
			if len(args) != 0 {
				return nil, cli.Usage("usage.invalid-arguments", "export takes flags only")
			}
			if book == "" || out == "" {
				return nil, cli.Usage("usage.missing-argument", "--book and --out are required")
			}
			payload, err := runExport(env, book, out, env.Globals.DryRun || check, check)
			if err != nil {
				return nil, cli.AsError(err)
			}
			return &cli.Result{Data: payload}, nil
		},
	}
}

type idPair struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type importPayload struct {
	DryRun     bool     `json:"dryRun"`
	Action     string   `json:"action"`
	Book       string   `json:"book"`
	Source     string   `json:"source,omitempty"`
	IDCount    int      `json:"idCount"`
	IDs        []idPair `json:"ids,omitempty"`
	Members    int      `json:"members"`
	Renamed    bool     `json:"renamed,omitempty"`
	Normalized bool     `json:"normalized,omitempty"`
	Receipt    string   `json:"receipt,omitempty"`
	Inbound    string   `json:"inbound,omitempty"`
}

func (payload importPayload) DryRunResult() bool { return payload.DryRun }

func (payload importPayload) Text() string {
	line := fmt.Sprintf("%s: imported %d member(s), %d fresh ID(s)", payload.Book,
		payload.Members, payload.IDCount)
	if payload.Renamed {
		line += fmt.Sprintf(" from %s", payload.Source)
	}
	if payload.Normalized {
		line += " (text normalized)"
	}
	if payload.Inbound != "" {
		line += " (inbound links unverified)"
	}
	return line + "\n"
}

func importCommand() *cli.Command {
	var from, as, target string
	var library, verify bool
	return &cli.Command{
		Name:     "import",
		Summary:  "import a text directory as a new book with fresh IDs",
		Usage:    "pecia import --from <dir> --as <book-id> [--path <dir>] [--library] [--verify]",
		Mutating: true,
		Flags: func(env *cli.Env) *flag.FlagSet {
			set := env.NewFlagSet("import")
			set.StringVar(&from, "from", "", "export directory")
			set.StringVar(&as, "as", "", "target book id")
			set.StringVar(&target, "path", "", "output path of the imported book")
			set.BoolVar(&library, "library", false, "register the book as the library")
			set.BoolVar(&verify, "verify", false, "verify the member hashes of the export")
			return set
		},
		Run: func(env *cli.Env, args []string) (*cli.Result, error) {
			if len(args) != 0 {
				return nil, cli.Usage("usage.invalid-arguments", "import takes flags only")
			}
			if from == "" || as == "" {
				return nil, cli.Usage("usage.missing-argument", "--from and --as are required")
			}
			payload, err := runImport(env, from, as, target, library, verify)
			if err != nil {
				return nil, cli.AsError(err)
			}
			return &cli.Result{Data: payload}, nil
		},
	}
}

// transferContext is the resolved environment of one transfer command.
type transferContext struct {
	Values    *config.Values
	Life      *lifecycle.Lifecycle
	Layout    store.Layout
	Allocator *idspace.Allocator
}

func openTransfer(env *cli.Env) (*transferContext, error) {
	values, configErr := config.Load(env)
	if configErr != nil {
		return nil, configErr
	}
	life, layout, lifeErr := lifecycle.FromEnv(env)
	if lifeErr != nil {
		return nil, lifeErr
	}
	return &transferContext{
		Values: values, Life: life, Layout: layout,
		Allocator: idspace.New(life, layout),
	}, nil
}

// bookID parses and validates a target book id.
func bookID(raw string) (model.BookID, error) {
	id := model.BookID(strings.TrimSpace(raw))
	if err := store.ValidateBookID(id); err != nil {
		return "", fail("transfer.invalid-book-id", "%v", err)
	}
	return id, nil
}

// outputPath resolves the registry output path of a transferred book.
func (context *transferContext) outputPath(id model.BookID, requested string, library bool) (string, error) {
	if library {
		if requested != "" {
			return "", fail("transfer.invalid-path",
				"the library book has the fixed output path \".\"")
		}
		return ".", nil
	}
	if requested != "" {
		clean, err := store.RegistryPath(requested)
		if err != nil {
			return "", fail("transfer.invalid-path", "%v", err)
		}
		return clean, nil
	}
	docs := context.Values.DocsRoot
	if filepath.IsAbs(docs) {
		if relative, err := filepath.Rel(context.Values.ProjectDir, docs); err == nil {
			docs = relative
		}
	}
	return path.Join(filepath.ToSlash(docs), string(id)), nil
}

// readManifest reads the manifest of a registered book, open or closed.
func readManifest(life *lifecycle.Lifecycle, layout store.Layout, id model.BookID) (*model.Manifest, error) {
	if paths, err := life.RequireOpen(id); err == nil {
		data, readErr := os.ReadFile(filepath.Join(paths.Dir, manifestMember))
		if readErr != nil {
			return nil, fail("transfer.read-failed", "%v", readErr)
		}
		return model.Decode(data)
	}
	paths, err := layout.Book(id)
	if err != nil {
		return nil, err
	}
	container, loadErr := store.Load(paths.Container)
	if loadErr != nil {
		return nil, loadErr
	}
	data, ok := container.Read(manifestMember)
	if !ok {
		return nil, fail("transfer.manifest-missing", "book %q has no manifest", id)
	}
	return model.Decode(data)
}

// writeReceipt records one transfer under the store's imports directory.
func (context *transferContext) writeReceipt(receipt map[string]any) (string, error) {
	now := time.Now().UTC()
	id, _ := receipt["targetBook"].(string)
	name := now.Format("20060102T150405Z") + "-" + id + ".json"
	path := filepath.Join(context.Layout.ImportsRoot(), name)
	receipt["at"] = now.Format(time.RFC3339)
	receipt["clabokVersion"] = buildinfo.Version
	data, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return "", fail("transfer.receipt-failed", "%v", err)
	}
	if err := store.WriteAtomic(path, append(data, '\n')); err != nil {
		return "", err
	}
	return path, nil
}
