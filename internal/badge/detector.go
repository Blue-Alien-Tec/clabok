package badge

import (
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

// Detector kinds. Every kind is declarative except command and http, which are opt-in and never
// evaluated by render or sync.
const (
	KindFile    = "file"
	KindExists  = "exists"
	KindCount   = "count"
	KindPackage = "package"
	KindGit     = "git"
	KindBuild   = "build"
	KindStatic  = "static"
	KindCommand = "command"
	KindHTTP    = "http"
)

// Query formats of a file detector.
const (
	FormatText  = "text"
	FormatRegex = "regex"
	FormatJSON  = "json"
	FormatYAML  = "yaml"
	FormatTOML  = "toml"
	FormatXML   = "xml"
)

// Reserved built-in detector ids. A config detector may not redefine one.
var builtinIDs = []string{"go", "module", "license", "remote", "version"}

// Detector describes one declarative badge source. It is configuration, not book data.
type Detector struct {
	Kind    string   `json:"kind"`
	Path    string   `json:"path,omitempty"`
	Format  string   `json:"format,omitempty"`
	Pattern string   `json:"pattern,omitempty"`
	Query   string   `json:"query,omitempty"`
	Value   string   `json:"value,omitempty"`
	Command []string `json:"command,omitempty"`
	URL     string   `json:"url,omitempty"`
	// Ecosystem and Field select a package manifest.
	Ecosystem string            `json:"ecosystem,omitempty"`
	Field     string            `json:"field,omitempty"`
	Map       map[string]string `json:"map,omitempty"`
	Prefix    string            `json:"prefix,omitempty"`
	Suffix    string            `json:"suffix,omitempty"`
	// Presentation defaults; a badge may override each one.
	Label string `json:"label,omitempty"`
	Color string `json:"color,omitempty"`
	Link  string `json:"link,omitempty"`
	Logo  string `json:"logo,omitempty"`
	Style string `json:"style,omitempty"`
}

// BuiltinID reports whether an id belongs to the built-in detector table.
func BuiltinID(id string) bool {
	for _, candidate := range builtinIDs {
		if candidate == id {
			return true
		}
	}
	return false
}

// ValidateDetector reports the first structural problem of a detector. The secret rules reject an
// external source before it can carry a credential.
func ValidateDetector(id string, detector Detector) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("the detector id is empty")
	}
	switch detector.Kind {
	case KindFile:
		if !safeRelative(detector.Path) {
			return fmt.Errorf("%s: path must stay inside the project", id)
		}
		switch detector.Format {
		case "", FormatText:
		case FormatRegex:
			if strings.TrimSpace(detector.Pattern) == "" {
				return fmt.Errorf("%s: a regex detector needs a pattern", id)
			}
		case FormatJSON, FormatYAML, FormatTOML, FormatXML:
			if strings.TrimSpace(detector.Query) == "" {
				return fmt.Errorf("%s: a %s detector needs a query", id, detector.Format)
			}
			if _, err := compilePattern(detector); err != nil {
				return fmt.Errorf("%s: %v", id, err)
			}
		default:
			return fmt.Errorf("%s: unknown file format %q", id, detector.Format)
		}
		if detector.Format == FormatRegex {
			if _, err := compilePattern(detector); err != nil {
				return fmt.Errorf("%s: %v", id, err)
			}
		}
	case KindExists, KindCount:
		if !safeRelative(detector.Path) {
			return fmt.Errorf("%s: path must stay inside the project", id)
		}
	case KindPackage:
		switch detector.Ecosystem {
		case "node", "dotnet", "rust", "go":
		default:
			return fmt.Errorf("%s: unknown package ecosystem %q", id, detector.Ecosystem)
		}
		if strings.TrimSpace(detector.Field) == "" {
			return fmt.Errorf("%s: a package detector needs a field", id)
		}
	case KindGit:
		if detector.Query != "remote" && detector.Query != "branch" {
			return fmt.Errorf("%s: a git detector queries remote or branch", id)
		}
	case KindBuild:
		if detector.Query != "version" {
			return fmt.Errorf("%s: a build detector queries version", id)
		}
	case KindStatic:
		if strings.TrimSpace(detector.Value) == "" {
			return fmt.Errorf("%s: a static detector needs a value", id)
		}
	case KindCommand:
		if len(detector.Command) == 0 || strings.TrimSpace(detector.Command[0]) == "" {
			return fmt.Errorf("%s: a command detector needs an argv list", id)
		}
		switch strings.ToLower(detector.Command[0]) {
		case "env", "printenv", "set", "cmdkey":
			return fmt.Errorf("%s: %q is not a permitted command", id, detector.Command[0])
		}
		for _, argument := range detector.Command {
			if containsSecret(argument) {
				return fmt.Errorf("%s: the command carries a secret placeholder", id)
			}
		}
	case KindHTTP:
		parsed, err := url.Parse(detector.URL)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			return fmt.Errorf("%s: an http detector needs an https URL", id)
		}
		if parsed.User != nil {
			return fmt.Errorf("%s: the URL must not carry credentials", id)
		}
		for key := range parsed.Query() {
			lower := strings.ToLower(key)
			for _, secret := range []string{"token", "key", "secret", "password", "sig"} {
				if strings.Contains(lower, secret) {
					return fmt.Errorf("%s: the URL must not carry a %q parameter", id, key)
				}
			}
		}
		if containsSecret(detector.URL) {
			return fmt.Errorf("%s: the URL carries a secret placeholder", id)
		}
	default:
		return fmt.Errorf("%s: unknown detector kind %q", id, detector.Kind)
	}
	return nil
}

// safeRelative reports a path that stays inside the project and may carry a glob.
func safeRelative(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	cleaned := filepath.ToSlash(filepath.Clean(path))
	if strings.HasPrefix(cleaned, "/") || strings.HasPrefix(cleaned, "../") || cleaned == ".." {
		return false
	}
	for _, segment := range strings.Split(cleaned, "/") {
		if segment == ".." {
			return false
		}
	}
	return true
}

// compilePattern compiles the regex of a detector; a bad pattern is a configuration error.
func compilePattern(detector Detector) (*regexp.Regexp, error) {
	if strings.TrimSpace(detector.Pattern) == "" {
		return nil, nil
	}
	return regexp.Compile(detector.Pattern)
}

// containsSecret reports a credential placeholder or an environment expansion.
func containsSecret(raw string) bool {
	for _, marker := range []string{"env://", "vault://", "secret://", "op://", "${", "$("} {
		if strings.Contains(raw, marker) {
			return true
		}
	}
	return strings.Contains(raw, "$")
}
