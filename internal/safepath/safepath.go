// Package safepath provides path containment checks and restricted filesystem helpers.
package safepath

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// dirPerm is owner-only directory permission (rwx------).
var dirPerm = os.FileMode(0o700)

// filePerm is owner-only file permission (rw-------).
const filePerm = 0o600

// JoinUnder joins elems under base and verifies the result stays within base.
func JoinUnder(base string, elems ...string) (string, error) {
	base = filepath.Clean(base)
	parts := append([]string{base}, elems...)
	target := filepath.Clean(filepath.Join(parts...))
	return withinBase(base, target)
}

func withinBase(base, target string) (string, error) {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return "", fmt.Errorf("resolving path under %q: %w", base, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes base %q", target, base)
	}
	return target, nil
}

// MkdirAll creates a directory tree with owner-only permissions.
func MkdirAll(path string) error {
	return os.MkdirAll(path, dirPerm)
}

// ReadFile reads a file after verifying it lies within base.
func ReadFile(base, filePath string) ([]byte, error) {
	safe, err := withinBase(filepath.Clean(base), filepath.Clean(filePath))
	if err != nil {
		return nil, err
	}
	return os.ReadFile(safe)
}

// WriteFile writes a file with owner-only permissions after verifying it lies within base.
func WriteFile(base, filePath string, data []byte) error {
	safe, err := withinBase(filepath.Clean(base), filepath.Clean(filePath))
	if err != nil {
		return err
	}
	if err := MkdirAll(filepath.Dir(safe)); err != nil {
		return err
	}
	return os.WriteFile(safe, data, filePerm)
}
