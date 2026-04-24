package util

import (
	"fmt"
	"path/filepath"
	"strings"
)

// SafePathUnderBase joins baseDir and name, resolves symlinks, and verifies the result
// is under baseDir. Returns the resolved path or an error if path traversal is detected.
// Use when baseDir may come from env vars and name from validated user input.
func SafePathUnderBase(baseDir, name string) (string, error) {
	baseClean := filepath.Clean(baseDir)
	nameClean := filepath.Clean(name)
	if nameClean == ".." || strings.HasPrefix(nameClean, "..") {
		return "", fmt.Errorf("path component contains traversal")
	}
	joined := filepath.Join(baseClean, nameClean)

	absBase, err := filepath.Abs(baseClean)
	if err != nil {
		return "", fmt.Errorf("resolve base: %w", err)
	}
	absBase, err = filepath.EvalSymlinks(absBase)
	if err != nil {
		return "", fmt.Errorf("resolve base symlinks: %w", err)
	}

	absPath, err := filepath.Abs(joined)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	absPath, err = filepath.EvalSymlinks(absPath)
	if err != nil {
		return "", fmt.Errorf("resolve path symlinks: %w", err)
	}

	rel, err := filepath.Rel(absBase, absPath)
	if err != nil {
		return "", fmt.Errorf("path not under base: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes base directory")
	}

	return absPath, nil
}
