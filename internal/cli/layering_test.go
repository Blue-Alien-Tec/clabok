package cli_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDomainPackagesDoNotImportCLI guards the dependency direction for the pure packages: the domain
// never imports the CLI or the entry point. Packages that also expose their own command surface
// (config, lifecycle, structure, coordinator) import the CLI by design. Packages that do not exist
// yet are skipped, so the list can grow with the scopes.
func TestDomainPackagesDoNotImportCLI(t *testing.T) {
	pure := []string{
		"buildinfo",
		"jsonstrict",
		"model",
		"store",
		"rollback",
		"render",
		"refs",
		"library",
		"asciiui",
	}
	for _, name := range pure {
		directory := filepath.Join("..", name)
		if _, err := os.Stat(directory); err != nil {
			continue
		}
		files := token.NewFileSet()
		packages, err := parser.ParseDir(files, directory, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("%s: %v", directory, err)
		}
		for packageName, parsed := range packages {
			for fileName, file := range parsed.Files {
				for _, imported := range file.Imports {
					path := strings.Trim(imported.Path.Value, `"`)
					if strings.HasSuffix(path, "/internal/cli") || strings.Contains(path, "/cmd/") {
						t.Fatalf("%s (%s) imports %s; the domain must not depend on the CLI",
							fileName, packageName, path)
					}
				}
			}
		}
	}
}
