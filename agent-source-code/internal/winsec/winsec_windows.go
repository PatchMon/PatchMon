//go:build windows

// Package winsec hardens Windows directories that hold agent secrets or staged
// executables, locking them to SYSTEM and Administrators so no other local user
// can read credentials or swap a payload before it runs.
package winsec

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

const trustedInstallerSID = "S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464"

// privateDirSDDL: owner Administrators, protected DACL, full control for SYSTEM
// and Administrators only, inherited by everything below.
const privateDirSDDL = "O:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)"

func dataDir() string {
	base := os.Getenv("ProgramData")
	if base == "" {
		base = `C:\ProgramData`
	}
	return filepath.Join(base, "PatchMon")
}

// EnsureDataDir creates %ProgramData%\PatchMon locked to SYSTEM and
// Administrators, or re-applies that lock to the existing directory so installs
// that predate the hardening self-heal on the next service start.
func EnsureDataDir() error {
	return secureDir(dataDir())
}

// secureDir creates dir granting full control to SYSTEM and Administrators only,
// or heals an existing plain directory to that state. The parent must exist. A
// reparse point in dir's place is refused, never followed.
func secureDir(dir string) error {
	switch err := CreatePrivateDir(dir); {
	case err == nil:
		return nil
	case errors.Is(err, windows.ERROR_ALREADY_EXISTS):
	default:
		return fmt.Errorf("create %s: %w", dir, err)
	}
	if !IsPlainDir(dir) {
		return fmt.Errorf("%s is a link or not a directory", dir)
	}
	if err := applyPrivateDir(dir); err != nil {
		return fmt.Errorf("harden %s: %w", dir, err)
	}
	return VerifySecureDir(dir)
}

// SecureUpdateDir returns %ProgramData%\PatchMon\update, a directory only SYSTEM
// and Administrators can write, for staging the replacement binary and update
// script. The service runs as LocalSystem, so a predictable user-writable
// staging path lets a local user swap the payload between its hash check and
// launch, landing as SYSTEM. An existing update directory that fails the
// owner/DACL/no-reparse check is deleted and recreated rather than trusted.
func SecureUpdateDir() (string, error) {
	if err := EnsureDataDir(); err != nil {
		return "", err
	}
	parent := dataDir()
	if err := VerifyOwner(parent); err != nil {
		return "", fmt.Errorf("%s: %w", parent, err)
	}
	dir := filepath.Join(parent, "update")
	switch err := CreatePrivateDir(dir); {
	case err == nil:
	case errors.Is(err, windows.ERROR_ALREADY_EXISTS):
		if !IsPlainDir(dir) || VerifySecureDir(dir) != nil {
			// RemoveAll deletes a reparse point itself without descending into
			// its target, so a planted junction cannot redirect the delete.
			if rmErr := os.RemoveAll(dir); rmErr != nil {
				return "", fmt.Errorf("remove insecure %s: %w", dir, rmErr)
			}
			if crErr := CreatePrivateDir(dir); crErr != nil {
				return "", fmt.Errorf("recreate %s: %w", dir, crErr)
			}
		}
	default:
		return "", fmt.Errorf("create %s: %w", dir, err)
	}
	if !IsPlainDir(dir) {
		return "", fmt.Errorf("%s is a link or not a directory", dir)
	}
	if err := VerifySecureDir(dir); err != nil {
		return "", fmt.Errorf("%s failed verification: %w", dir, err)
	}
	return dir, nil
}

// IsPlainDir reports whether path is a real directory and not a symlink,
// junction or other reparse point.
func IsPlainDir(path string) bool {
	fi, err := os.Lstat(path)
	return err == nil && fi.IsDir() && fi.Mode()&(os.ModeSymlink|os.ModeIrregular) == 0
}

// CreatePrivateDir creates path with its owner and DACL set atomically from
// privateDirSDDL. It fails with ERROR_ALREADY_EXISTS rather than adopting a
// directory someone else created, and its parent must already exist.
func CreatePrivateDir(path string) error {
	sd, err := windows.SecurityDescriptorFromString(privateDirSDDL)
	if err != nil {
		return err
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	return windows.CreateDirectory(p, &sa)
}

// applyPrivateDir re-applies privateDirSDDL's owner and protected DACL to an
// existing directory, detaching it from inheritance so a permissive parent
// cannot widen it again.
func applyPrivateDir(path string) error {
	sd, err := windows.SecurityDescriptorFromString(privateDirSDDL)
	if err != nil {
		return err
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		owner, nil, dacl, nil,
	)
}

func isPrivilegedSID(sid *windows.SID) bool {
	return sid != nil && (sid.IsWellKnown(windows.WinLocalSystemSid) ||
		sid.IsWellKnown(windows.WinBuiltinAdministratorsSid) ||
		sid.String() == trustedInstallerSID)
}

// VerifyOwner reports an error unless path is owned by SYSTEM, Administrators or
// TrustedInstaller.
func VerifyOwner(path string) error {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return err
	}
	if !isPrivilegedSID(owner) {
		return fmt.Errorf("owned by %s", owner.String())
	}
	return nil
}

// VerifySecureDir reports an error unless path is owned by SYSTEM,
// Administrators or TrustedInstaller, has a protected DACL, and grants access to
// no one else.
func VerifySecureDir(path string) error {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	return checkPrivateDescriptor(sd)
}

func checkPrivateDescriptor(sd *windows.SECURITY_DESCRIPTOR) error {
	owner, _, err := sd.Owner()
	if err != nil {
		return err
	}
	if !isPrivilegedSID(owner) {
		return fmt.Errorf("owned by %s", owner.String())
	}
	control, _, err := sd.Control()
	if err != nil {
		return err
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		return errors.New("its permissions are inherited")
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	if dacl == nil {
		return errors.New("it has no DACL, so everyone has access")
	}
	for i := uint32(0); i < uint32(dacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, i, &ace); err != nil {
			return err
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0 {
			continue
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if !isPrivilegedSID(sid) {
			return fmt.Errorf("it grants access to %s", sid.String())
		}
	}
	return nil
}
