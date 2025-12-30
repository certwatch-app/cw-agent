// Package version provides build-time version information.
package version

import (
	"runtime"
)

// Build-time variables (set via ldflags)
var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

// Info contains version information
type Info struct {
	Version   string
	GitCommit string
	BuildDate string
	GoVersion string
	OS        string
	Arch      string
}

// GetInfo returns the full version information
func GetInfo() Info {
	return Info{
		Version:   Version,
		GitCommit: GitCommit,
		BuildDate: BuildDate,
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}
}

// GetVersion returns just the version string
func GetVersion() string {
	return Version
}
