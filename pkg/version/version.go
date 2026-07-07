// Package version holds build metadata, injected via -ldflags at build time.
package version

var (
	// Version is the semantic version (or git describe) of this build.
	Version = "dev"
	// GitCommit is the short commit hash of this build.
	GitCommit = "unknown"
)
