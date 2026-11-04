package pkgversion

// SigningPublicKey is the Ed25519 public key used to verify agent update binaries.
// It must be set at build time via the PATCHMON_SIGNING_PUBLIC_KEY environment variable:
//
//	go build \
//	  -ldflags="-X patchmon-agent/internal/pkgversion.SigningPublicKey=${PATCHMON_SIGNING_PUBLIC_KEY}" \
//	  ./cmd/patchmon-agent
//
// A build without this variable will refuse to perform updates.
var SigningPublicKey = ""
