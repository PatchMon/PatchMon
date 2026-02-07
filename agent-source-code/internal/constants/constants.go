package constants

// SELinux status constants
const (
	SELinuxEnabled    = "enabled"
	SELinuxDisabled   = "disabled"
	SELinuxPermissive = "permissive"
	SELinuxEnforcing  = "enforcing" // Will be mapped to enabled for API compatibility
)

// Note: OS type detection uses string literals directly in system package
// These constants are reserved for future use if needed

// Architecture constants
const (
	ArchX86_64  = "x86_64"
	ArchAMD64   = "amd64"
	ArchARM64   = "arm64"
	ArchAARCH64 = "aarch64"
	ArchUnknown = "arch_unknown"
)

// Network interface types
const (
	NetTypeEthernet = "ethernet"
	NetTypeWiFi     = "wifi"
	NetTypeBridge   = "bridge"
)

// IP address families
const (
	IPFamilyIPv4 = "inet"
	IPFamilyIPv6 = "inet6"
)

// Repository type constants
const (
	RepoTypeDeb    = "deb"
	RepoTypeDebSrc = "deb-src"
	RepoTypeRPM    = "rpm"
	RepoTypeAPK    = "apk"
)

// Log level constants
const (
	LogLevelDebug = "debug"
	LogLevelInfo  = "info"
	LogLevelWarn  = "warn"
	LogLevelError = "error"
)

// Common error messages
const (
	ErrUnknownValue = "Unknown"
)
