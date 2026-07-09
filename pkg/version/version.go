// Package version holds build metadata, injected via -ldflags at build time.
package version

import "runtime/debug"

var (
	// Version is the semantic version (or git describe) of this build.
	Version = "dev"
	// GitCommit is the short commit hash of this build.
	GitCommit = "unknown"
)

// When built without -ldflags (e.g. `go install ...@v0.1.0`), fall back to the
// module version and VCS revision embedded by the Go toolchain so the binary
// still reports a meaningful version.
func init() {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	if Version == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
		Version = info.Main.Version
	}
	if GitCommit == "unknown" {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" && s.Value != "" {
				if len(s.Value) > 12 {
					GitCommit = s.Value[:12]
				} else {
					GitCommit = s.Value
				}
				break
			}
		}
	}
}
