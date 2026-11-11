// Package buildinfo carries the product identity and the development version.
//
// Version is the development version and the single source of truth in the tree. Release builds
// override it with -ldflags "-X github.com/Blue-Alien-Tec/clabok/internal/buildinfo.Version=<tag>".
// A change to this file is the only verify-CI trigger.
package buildinfo

const (
	// Product is the product name used in user-facing output.
	Product = "CLABOK"
	// CLI is the executable name.
	CLI = "pecia"
)

// Version is the development version; the milestone close bumps it.
var Version = "1.1.2"
