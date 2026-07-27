// Package testutil provides shared test helpers for the juggernaut codebase.
//
// All test files in cmd/, internal/config, internal/provider, internal/activation,
// and internal/keychain should import this package instead of defining their
// own copies of NewTestHome, NestedMapChain, CaptureStdout, etc.
package testutil

import (
	"bytes"
	"io"
	"os"
	"testing"
	"time"

	"github.com/jpvelasco/juggernaut/v5/internal/keychain"
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

// NestedMapChain navigates a chain of keys through nested map[string]any
// structures. The final key returns its raw value (not asserted as map).
// Returns (value, true) if every intermediate level is a map and the leaf
// exists; (nil, false) otherwise.
//
// Usage: NestedMapChain(plan.Keys, "model_providers", "amazon-bedrock", "aws", "region")
func NestedMapChain(root map[string]any, chain ...string) (any, bool) {
	current := root
	for i, k := range chain {
		val, ok := current[k]
		if !ok {
			return nil, false
		}
		// Last key — return the raw value regardless of type.
		if i == len(chain)-1 {
			return val, true
		}
		m, ok := val.(map[string]any)
		if !ok {
			return nil, false
		}
		current = m
	}
	return nil, false
}

// CaptureStdout runs fn while redirecting os.Stdout and returns what was printed.
func CaptureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("closing pipe: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("reading pipe: %v", err)
	}
	return string(out)
}

// WithStdin replaces os.Stdin with a pipe preloaded with input for the duration
// of fn, so interactive confirmation prompts can be exercised in tests.
func WithStdin(t *testing.T, input string, fn func()) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	if _, err := w.WriteString(input); err != nil {
		t.Fatalf("writing stdin: %v", err)
	}
	_ = w.Close() // best-effort; test validity depends on reader side

	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = orig })
	fn()
}

// Buffer is a convenience wrapper around bytes.Buffer for test assertions.
func Buffer() *bytes.Buffer {
	return new(bytes.Buffer)
}

// SkipIfNoKeychain probes the default keychain store and skips the test if
// the backend is unavailable (e.g. headless Linux CI with no Secret Service).
// The probe is bounded by a 3-second timeout to prevent blocking.
// Sets JUGGERNAUT_KEYCHAIN_SERVICE to "jug-testutil" for isolation.
func SkipIfNoKeychain(t *testing.T) *keychain.Store {
	t.Helper()
	t.Setenv("JUGGERNAUT_KEYCHAIN_SERVICE", "jug-testutil")
	store := keychain.Default()
	done := make(chan error, 1)
	go func() { done <- store.Set("probe") }()
	select {
	case err := <-done:
		if err != nil {
			t.Skipf("keychain backend unavailable: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Skip("keychain backend timed out")
	}
	_ = store.Delete()
	return store
}
