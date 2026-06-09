// Package version holds build metadata injected at link time.
package version

// Set via -ldflags at build/release time.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)
