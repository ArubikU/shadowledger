// Package version reports the build version. The release workflow injects the
// git tag via -ldflags "-X .../version.Version=vX.Y.Z"; dev builds say "dev".
package version

// Version is the build version (overridden at link time on releases).
var Version = "dev"

// Repo is the canonical source repository (used for update checks).
const Repo = "ArubikU/shadowledger"
