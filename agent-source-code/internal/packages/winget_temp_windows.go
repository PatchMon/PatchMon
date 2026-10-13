//go:build windows

package packages

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const trustedInstallerSID = "S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464"

const privateDirPrefix = "pm-"

// wingetTempDir returns the directory winget gets as TEMP. Only SystemTemp, or a container
// the agent created itself, is used, and either must pass verifySecureDir: a folder a user
// made first keeps that user's ownership whatever DACL is applied over it.
func wingetTempDir() (string, error) {
	if root := os.Getenv("SystemRoot"); root != "" {
		sys := filepath.Join(root, "SystemTemp")
		if isPlainDir(sys) && verifySecureDir(sys) == nil {
			return sys, nil
		}
	}
	base := os.Getenv("ProgramData")
	if base == "" {
		base = `C:\ProgramData`
	}
	parent := filepath.Join(base, "PatchMon")
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return "", err
	}
	if !isPlainDir(parent) {
		return "", errors.New(parent + " is a link or not a directory")
	}
	if err := verifyOwner(parent); err != nil {
		return "", fmt.Errorf("%s: %w", parent, err)
	}
	dir := filepath.Join(parent, "winget-tmp")
	if err := createPrivateDir(dir); err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return "", fmt.Errorf("create %s: %w", dir, err)
	}
	if !isPlainDir(dir) {
		return "", errors.New(dir + " is a link or not a directory")
	}
	if err := verifySecureDir(dir); err != nil {
		return "", fmt.Errorf("%s was not created by the agent and will not be used: %w", dir, err)
	}
	return dir, nil
}

// newRunTempDir creates a fresh, randomly named private directory under root for one
// winget upgrade, so an installer is never staged at a path a user could predict.
func newRunTempDir(root string) (string, error) {
	removeStaleRunDirs(root, 24*time.Hour)
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	dir := filepath.Join(root, privateDirPrefix+hex.EncodeToString(b[:]))
	if err := createPrivateDir(dir); err != nil {
		return "", fmt.Errorf("create %s: %w", dir, err)
	}
	return dir, nil
}

func removeStaleRunDirs(root string, age time.Duration) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), privateDirPrefix) {
			continue
		}
		if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > age {
			_ = os.RemoveAll(filepath.Join(root, e.Name()))
		}
	}
}

func isPlainDir(path string) bool {
	fi, err := os.Lstat(path)
	return err == nil && fi.IsDir() && fi.Mode()&(os.ModeSymlink|os.ModeIrregular) == 0
}

// privateDirSDDL: owner Administrators, protected DACL, full control for SYSTEM and
// Administrators only, inherited by everything below.
const privateDirSDDL = "O:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)"

// createPrivateDir creates path with its owner and DACL set atomically. It fails with
// ERROR_ALREADY_EXISTS rather than adopting a directory someone else created.
func createPrivateDir(path string) error {
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

func isPrivilegedSID(sid *windows.SID) bool {
	return sid != nil && (sid.IsWellKnown(windows.WinLocalSystemSid) ||
		sid.IsWellKnown(windows.WinBuiltinAdministratorsSid) ||
		sid.String() == trustedInstallerSID)
}

func verifyOwner(path string) error {
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

// verifySecureDir checks that path is owned by SYSTEM, Administrators or TrustedInstaller,
// has a protected DACL, and grants access to no one else.
func verifySecureDir(path string) error {
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
