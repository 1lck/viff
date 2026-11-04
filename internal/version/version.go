// Package version carries the build identity of this Viff binary. The values
// are set at build time with -ldflags -X; a plain "go build" keeps the
// development defaults, so an unreleased build is still identifiable.
package version

import (
	"runtime/debug"
	"strings"
)

// Set by the build:
//
//	go build -ldflags "-X viff/internal/version.Version=v0.1.0-alpha.3 \
//	  -X viff/internal/version.Commit=$(git rev-parse --short HEAD) \
//	  -X viff/internal/version.BuildDate=2026-11-04"
var (
	// Version is the release version, such as v0.1.0-alpha.3 or dev.
	Version = "dev"
	// Commit is the short commit the binary was built from.
	Commit = ""
	// BuildDate is the UTC date of the build.
	BuildDate = ""
)

// String returns the one-line build identity, omitting empty fields:
// "v0.1.0-alpha.3 (abc1234, 2026-11-04)".
func String() string {
	parts := make([]string, 0, 3)
	if Commit != "" {
		parts = append(parts, Commit)
	}
	if BuildDate != "" {
		parts = append(parts, BuildDate)
	}
	if len(parts) == 0 {
		return Version
	}
	return Version + " (" + strings.Join(parts, ", ") + ")"
}

// VivaVersion returns the Viva module version recorded in the build, or
// "unknown" when the binary has no module metadata.
func VivaVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, dep := range info.Deps {
		if dep.Path == "github.com/vivaui/viva" {
			return dep.Version
		}
	}
	return "unknown"
}

// GoVersion returns the Go toolchain that built the binary.
func GoVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	return info.GoVersion
}
