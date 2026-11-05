// Package buildinfo carries the product identity and the injected build version.
//
// The version is set at build time with
// -ldflags "-X github.com/Blue-Alien-Tec/clabok/internal/buildinfo.Version=<version>".
package buildinfo

const (
	// Product is the product name used in user-facing output.
	Product = "CLABOK"
	// CLI is the executable name.
	CLI = "pecia"
)

// Version is the build version; the development default applies to unstamped builds.
var Version = "0.1.0-dev"
