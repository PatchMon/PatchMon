// Package pkgversion provides version information for the agent
package pkgversion

// Version is the agent version. It is a var rather than a const so builds can
// override it at link time:
//
//	-ldflags "-X patchmon-agent/internal/pkgversion.Version=2.0.4"
//
// The value below is a deliberate non-version. Every supported build path
// (make, docker/build.sh, CI) injects the real version derived from the git
// tag, so seeing 0.0.0 in a UI means a build path skipped the injection.
var Version = "0.0.0"
