// Package badge resolves repository facts into the badges of the library README. A declared badge
// names a detector; an explicit message overrides the detector. Nothing here writes.
package badge

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/Blue-Alien-Tec/clabok/internal/buildinfo"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

// Value is one resolved badge.
type Value struct {
	ID         string
	Detector   string
	Label      string
	Message    string
	Link       string
	Source     string
	Color      string
	Style      string
	Logo       string
	LogoColor  string
	LabelColor string
	Alt        string
	Required   bool
	Order      int
	Group      string
	// URL carries the rendered image URL of an http source; Dynamic marks a value that comes from a
	// cached or remote source instead of a repository fact.
	URL     string
	Dynamic bool
}

// Unresolved reports one declared badge without a value.
type Unresolved struct {
	ID     string
	Reason string
}

type detector func(projectDir string) (Value, bool, string)

type descriptor struct {
	label string
	color string
}

var descriptors = map[string]descriptor{
	"go":      {label: "go", color: "00ADD8"},
	"module":  {label: "module", color: "lightgrey"},
	"license": {label: "license", color: "blue"},
	"remote":  {label: "remote", color: "lightgrey"},
	"version": {label: "version", color: "green"},
}

var detectors = map[string]detector{
	"go":      goDirective,
	"module":  moduleDirective,
	"license": licenseFile,
	"remote":  remoteURL,
	"version": versionValue,
}

// Resolve returns the concrete value for one declared badge. ok is false when neither an explicit
// message nor the detector supplies a value; reason then explains the omission.
// Resolver.Resolve carries the configuration and cache; this package function is the built-in-only
// shorthand used by tests.
func Resolve(projectDir string, declared model.Badge) (Value, bool, string) {
	return Resolver{ProjectDir: projectDir}.Resolve(declared)
}

// ResolveAll resolves the declared badges with the built-in detectors only.
func ResolveAll(projectDir string, badges []model.Badge) ([]Value, []Unresolved) {
	return Resolver{ProjectDir: projectDir}.ResolveAll(badges)
}

// BadgeURL returns the shields.io URL for a value, or the declared URL of an http source.
func BadgeURL(value Value) string {
	if value.URL != "" {
		return value.URL
	}
	color := value.Color
	if color == "" {
		color = "lightgrey"
	}
	builder := fmt.Sprintf("https://img.shields.io/badge/%s-%s-%s?style=%s",
		escape(value.Label), escape(value.Message), color, url.QueryEscape(styleOf(value.Style)))
	if value.Logo != "" {
		builder += "&logo=" + url.QueryEscape(value.Logo)
	}
	if value.LogoColor != "" {
		builder += "&logoColor=" + url.QueryEscape(value.LogoColor)
	}
	if value.LabelColor != "" {
		builder += "&labelColor=" + url.QueryEscape(value.LabelColor)
	}
	return builder
}

func styleOf(style string) string {
	if style == "" {
		return "flat-square"
	}
	return style
}

// AltText is the Markdown image alt text of a badge.
func AltText(value Value) string {
	if value.Alt != "" {
		return value.Alt
	}
	return value.Label + ": " + value.Message
}

// escape encodes one shields.io path segment: a literal dash is doubled and a space becomes an
// underscore before the segment is URL-escaped.
func escape(text string) string {
	text = strings.ReplaceAll(text, "-", "--")
	text = strings.ReplaceAll(text, " ", "_")
	return url.PathEscape(text)
}

func goDirective(projectDir string) (Value, bool, string) {
	text, ok := readText(projectDir, "go.mod")
	if !ok {
		return Value{}, false, "go.mod is missing"
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "go ") {
			version := strings.TrimSpace(strings.TrimPrefix(line, "go "))
			if version != "" {
				return Value{Message: version, Link: "https://go.dev", Source: "go.mod"}, true, ""
			}
		}
	}
	return Value{}, false, "go.mod has no go directive"
}

func moduleDirective(projectDir string) (Value, bool, string) {
	text, ok := readText(projectDir, "go.mod")
	if !ok {
		return Value{}, false, "go.mod is missing"
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			path := strings.TrimSpace(strings.TrimPrefix(line, "module "))
			if path != "" {
				return Value{Message: path, Source: "go.mod"}, true, ""
			}
		}
	}
	return Value{}, false, "go.mod has no module directive"
}

func licenseFile(projectDir string) (Value, bool, string) {
	for _, name := range []string{"LICENSE", "LICENSE.md", "LICENSE.txt", "COPYING"} {
		text, ok := readText(projectDir, name)
		if !ok {
			continue
		}
		lower := strings.ToLower(text)
		switch {
		case strings.Contains(lower, "mit license"):
			return Value{Message: "MIT", Link: name, Source: name}, true, ""
		case strings.Contains(lower, "apache license"):
			return Value{Message: "Apache-2.0", Link: name, Source: name}, true, ""
		case strings.Contains(lower, "gnu general public license") && strings.Contains(lower, "version 3"):
			return Value{Message: "GPL-3.0", Link: name, Source: name}, true, ""
		case strings.Contains(lower, "gnu general public license") && strings.Contains(lower, "version 2"):
			return Value{Message: "GPL-2.0", Link: name, Source: name}, true, ""
		case strings.Contains(lower, "bsd 3-clause"):
			return Value{Message: "BSD-3-Clause", Link: name, Source: name}, true, ""
		default:
			return Value{Message: "custom", Link: name, Source: name}, true, ""
		}
	}
	return Value{}, false, "no LICENSE or COPYING file"
}

func remoteURL(projectDir string) (Value, bool, string) {
	text, ok := readText(projectDir, filepath.Join(".git", "config"))
	if !ok {
		return Value{}, false, ".git/config is missing"
	}
	url := originURL(text)
	if url == "" {
		return Value{}, false, ".git/config has no remote origin url"
	}
	message, link, valid := normalizeRemote(url)
	if !valid {
		return Value{}, false, fmt.Sprintf("remote origin url %q is not understood", url)
	}
	return Value{Message: message, Link: link, Source: ".git/config"}, true, ""
}

func versionValue(projectDir string) (Value, bool, string) {
	if buildinfo.Version == "" {
		return Value{}, false, "buildinfo version is empty"
	}
	return Value{Message: buildinfo.Version, Source: "internal/buildinfo"}, true, ""
}

// originURL finds the origin url in an INI-style git config.
func originURL(config string) string {
	inOrigin := false
	for _, line := range strings.Split(config, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			inOrigin = strings.EqualFold(trimmed, `[remote "origin"]`)
			continue
		}
		if inOrigin && strings.HasPrefix(strings.ToLower(trimmed), "url") {
			parts := strings.SplitN(trimmed, "=", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

// normalizeRemote turns an scp-like or URL remote into "owner/repo" plus an https link.
func normalizeRemote(raw string) (string, string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", false
	}
	var host, path string
	switch {
	case strings.HasPrefix(raw, "git@"):
		rest := strings.TrimPrefix(raw, "git@")
		parts := strings.SplitN(rest, ":", 2)
		if len(parts) != 2 {
			return "", "", false
		}
		host, path = parts[0], parts[1]
	case strings.Contains(raw, "://"):
		parts := strings.SplitN(raw, "://", 2)
		rest := parts[1]
		if at := strings.Index(rest, "@"); at >= 0 {
			rest = rest[at+1:]
		}
		slash := strings.Index(rest, "/")
		if slash < 0 {
			return "", "", false
		}
		host, path = rest[:slash], rest[slash+1:]
	default:
		return "", "", false
	}
	path = strings.TrimSuffix(path, ".git")
	path = strings.Trim(path, "/")
	if host == "" || path == "" {
		return "", "", false
	}
	return path, "https://" + host + "/" + path, true
}

func readText(projectDir, name string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(projectDir, filepath.FromSlash(name)))
	if err != nil {
		return "", false
	}
	return string(data), true
}
