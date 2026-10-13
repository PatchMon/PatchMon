//go:build windows

package packages

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func wingetTempDir() (string, error) {
	if root := os.Getenv("SystemRoot"); root != "" {
		sys := filepath.Join(root, "SystemTemp")
		if isPlainDir(sys) {
			return sys, nil
		}
	}
	base := os.Getenv("ProgramData")
	if base == "" {
		base = `C:\ProgramData`
	}
	parent := filepath.Join(base, "PatchMon")
	dir := filepath.Join(parent, "winget-tmp")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	if !isPlainDir(parent) || !isPlainDir(dir) {
		return "", errors.New(dir + " is a link or not a directory")
	}
	if err := protectSystemOnly(dir); err != nil {
		return "", fmt.Errorf("restrict %s: %w", dir, err)
	}
	return dir, nil
}

func isPlainDir(path string) bool {
	fi, err := os.Lstat(path)
	return err == nil && fi.IsDir() && fi.Mode()&(os.ModeSymlink|os.ModeIrregular) == 0
}

// protectSystemOnly makes Administrators the owner and gives SYSTEM and Administrators
// the only access, with inheritance from the parent cut, so a directory a user created
// first cannot stay writable by them.
func protectSystemOnly(path string) error {
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return err
	}
	admins, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return err
	}
	entries := make([]windows.EXPLICIT_ACCESS, 0, 2)
	for _, sid := range []*windows.SID{system, admins} {
		entries = append(entries, windows.EXPLICIT_ACCESS{
			AccessPermissions: windows.GENERIC_ALL,
			AccessMode:        windows.SET_ACCESS,
			Inheritance:       windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT,
			Trustee: windows.TRUSTEE{
				TrusteeForm:  windows.TRUSTEE_IS_SID,
				TrusteeType:  windows.TRUSTEE_IS_GROUP,
				TrusteeValue: windows.TrusteeValueFromSID(sid),
			},
		})
	}
	acl, err := windows.ACLFromEntries(entries, nil)
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		admins, nil, acl, nil,
	)
}
