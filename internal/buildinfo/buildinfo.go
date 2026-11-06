// Package buildinfo carries the product identity and the development version.
//
// Version is the single source of truth and is not stamped with -ldflags. A change to this file is
// the only CI trigger.
package buildinfo

const (
	// Product is the product name used in user-facing output.
	Product = "CLABOK"
	// CLI is the executable name.
	CLI = "pecia"
)

// Version is the development version; the milestone close bumps it.
var Version = "1.2.0-dev"
