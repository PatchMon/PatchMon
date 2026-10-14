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

	"patchmon-agent/internal/winsec"

	"golang.org/x/sys/windows"
)

const privateDirPrefix = "pm-"

// wingetTempDir returns the directory winget gets as TEMP. Only SystemTemp, or a container
// the agent created itself, is used, and either must pass VerifySecureDir: a folder a user
// made first keeps that user's ownership whatever DACL is applied over it.
func wingetTempDir() (string, error) {
	if root := os.Getenv("SystemRoot"); root != "" {
		sys := filepath.Join(root, "SystemTemp")
		if winsec.IsPlainDir(sys) && winsec.VerifySecureDir(sys) == nil {
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
	if !winsec.IsPlainDir(parent) {
		return "", errors.New(parent + " is a link or not a directory")
	}
	if err := winsec.VerifyOwner(parent); err != nil {
		return "", fmt.Errorf("%s: %w", parent, err)
	}
	dir := filepath.Join(parent, "winget-tmp")
	if err := winsec.CreatePrivateDir(dir); err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return "", fmt.Errorf("create %s: %w", dir, err)
	}
	if !winsec.IsPlainDir(dir) {
		return "", errors.New(dir + " is a link or not a directory")
	}
	if err := winsec.VerifySecureDir(dir); err != nil {
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
	if err := winsec.CreatePrivateDir(dir); err != nil {
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
