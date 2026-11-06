package model

import "strings"

// ValidRepoPath reports whether a repository-relative path is well formed. An empty path and "."
// denote the repository root.
func ValidRepoPath(path string) bool {
	if path == "" || path == "." {
		return true
	}
	if strings.HasPrefix(path, "/") || strings.HasSuffix(path, "/") || strings.Contains(path, "\\") {
		return false
	}
	for _, segment := range strings.Split(path, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

// LayoutConfig describes the generated repository-layout block. Root and Exclude are
// repository-relative paths; Depth limits the levels below Root, with 0 meaning the default. Notes
// carry the one-line prose of a path.
type LayoutConfig struct {
	Root    string            `json:"root,omitempty"`
	Depth   int               `json:"depth,omitempty"`
	Exclude []string          `json:"exclude,omitempty"`
	Notes   map[string]string `json:"notes,omitempty"`
}

// Clone copies the layout configuration.
func (config *LayoutConfig) Clone() *LayoutConfig {
	if config == nil {
		return nil
	}
	clone := &LayoutConfig{Root: config.Root, Depth: config.Depth}
	if config.Exclude != nil {
		clone.Exclude = append([]string(nil), config.Exclude...)
	}
	if config.Notes != nil {
		clone.Notes = make(map[string]string, len(config.Notes))
		for path, note := range config.Notes {
			clone.Notes[path] = note
		}
	}
	return clone
}
