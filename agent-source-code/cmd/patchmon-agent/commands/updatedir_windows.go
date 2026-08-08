//go:build windows

package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// secureUpdateDir returns a directory that only SYSTEM and Administrators can
// write to, for staging the replacement binary and the update script.
//
// The service runs as LocalSystem, so os.TempDir() resolves to C:\Windows\TEMP.
// Staging there and then executing from it gives anyone who can write to that
// directory a window in which to swap the payload, which lands as SYSTEM. Go
// also maps only the read-only attribute on Windows, so a 0600 file mode is not
// a control here: the DACL has to be set explicitly.
func secureUpdateDir() (string, error) {
	base := os.Getenv("ProgramData")
	if base == "" {
		base = `C:\ProgramData`
	}
	dir := filepath.Join(base, "PatchMon", "update")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create update directory: %w", err)
	}
	if err := restrictToSystemAndAdmins(dir); err != nil {
		return "", fmt.Errorf("secure update directory: %w", err)
	}
	return dir, nil
}

// restrictToSystemAndAdmins replaces the DACL with one granting full control to
// LocalSystem and Administrators only, and detaches it from inheritance so a
// permissive parent cannot widen it again.
func restrictToSystemAndAdmins(path string) error {
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
			AccessMode:        windows.GRANT_ACCESS,
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
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, acl, nil,
	)
}
