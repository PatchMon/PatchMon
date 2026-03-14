package agents

import _ "embed"

// PatchmonInstallScript is the embedded patchmon_install.sh script.
// It is served at GET /api/v1/hosts/install with env vars and fetch_credentials injected.
//
//go:embed patchmon_install.sh
var PatchmonInstallScript []byte

// DirectHostAutoEnrollScript is served at GET /api/v1/auto-enrollment/script?type=direct-host
//
//go:embed direct_host_auto_enroll.sh
var DirectHostAutoEnrollScript []byte

// ProxmoxAutoEnrollScript is served at GET /api/v1/auto-enrollment/script?type=proxmox-lxc
//
//go:embed proxmox_auto_enroll.sh
var ProxmoxAutoEnrollScript []byte

// PatchmonRemoveScript is the embedded patchmon_remove.sh script.
// It is served at GET /api/v1/hosts/remove (public, no auth required).
//
//go:embed patchmon_remove.sh
var PatchmonRemoveScript []byte

// PatchmonWindowsInstallScript is the embedded Windows PowerShell install script.
// Served at GET /api/v1/hosts/install?os=windows
//
//go:embed patchmon_install_windows.ps1
var PatchmonWindowsInstallScript []byte

// PatchmonWindowsRemoveScript is the embedded Windows PowerShell removal script.
// Served at GET /api/v1/hosts/remove?os=windows
//
//go:embed patchmon_remove_windows.ps1
var PatchmonWindowsRemoveScript []byte
