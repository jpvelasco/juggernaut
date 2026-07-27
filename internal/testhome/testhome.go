// Package testhome provides NewTestHome — a temp directory with HOME/USERPROFILE
// set. Deliberately depends on nothing internal, so every test package in the
// codebase can import it without triggering an import cycle.
package testhome

import (
	"testing"
)

// NewTestHome creates a temp directory and sets HOME/USERPROFILE to it.
// Replaces the pattern:
//
//	home := t.TempDir()
//	t.Setenv("HOME", home)
//	t.Setenv("USERPROFILE", home)
func NewTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}
