// Package version holds the build-time version string injected via ldflags.
package version

// Version is set at build time via -ldflags "-X coldmic/internal/version.Version=v0.1.0".
// When built without ldflags (e.g. go run), it defaults to "dev".
var Version = "dev"
