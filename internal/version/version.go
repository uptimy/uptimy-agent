package version

// Package version holds build-time version information injected via ldflags.

// Set via -ldflags at build time.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)
