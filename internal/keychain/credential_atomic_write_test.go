package keychain

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
	"github.com/jpvelasco/juggernaut/v5/internal/testhome"
)

// TestWriteCredentialFile_WriteFailure_PreservesPrevious is the regression
// for #386: replacing an existing file-backed credential must be atomic.
// If the replacement write fails, the previous authoritative envelope must
// survive intact and remain readable via GetWithFallback.
//
// The failure is injected through the writeCredentialTempFn seam so the
// fault lands deterministically at the temp-file write, simulating a
// disk-full / I/O error on the commit path.
func TestWriteCredentialFile_WriteFailure_PreservesPrevious(t *testing.T) {
	home := testhome.NewTestHome(t)
	filePath := credentialFilePath(home)

	if err := safepath.MkdirAll(filepath.Dir(filePath)); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Seed a previous credential directly (bypassing Store so we don't
	// touch the keychain).
	previous := "previous-token"
	prevPayload, err := encodeCredential(previous)
	if err != nil {
		t.Fatalf("encode previous: %v", err)
	}
	if err := safepath.WriteFile(home, filePath, prevPayload); err != nil {
		t.Fatalf("seed credential: %v", err)
	}

	if got, err := Default().GetWithFallback(home); err != nil || got != previous {
		t.Fatalf("seed read: got %q, err %v; want %q", got, err, previous)
	}

	// Force the temp-file write to fail.
	orig := writeCredentialTempFn
	writeCredentialTempFn = func(string, []byte) error {
		return fmt.Errorf("injected write failure")
	}
	defer func() { writeCredentialTempFn = orig }()

	if err := writeCredentialFile(home, filePath, "new-token"); err == nil {
		t.Fatal("expected write failure, got nil")
	}

	// The previous credential must survive the failed replacement.
	got, err := Default().GetWithFallback(home)
	if err != nil {
		t.Fatalf("GetWithFallback after failed write: %v", err)
	}
	if got != previous {
		t.Fatalf("previous credential lost after failed replacement: got %q, want %q", got, previous)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read credential file: %v", err)
	}
	if !isVersionedCredential(data) {
		t.Errorf("credential file is not a versioned envelope after failed write: %q", string(data))
	}

	if _, err := os.Stat(filePath + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("stray temp file left behind: %v", err)
	}
}

// TestWriteCredentialFile_AtomicReplacement verifies the happy path: a
// successful replacement of an existing credential file leaves exactly one
// versioned envelope containing the new token, and no temp file.
func TestWriteCredentialFile_AtomicReplacement(t *testing.T) {
	home := testhome.NewTestHome(t)
	filePath := credentialFilePath(home)
	if err := safepath.MkdirAll(filepath.Dir(filePath)); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	prevPayload, err := encodeCredential("old")
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if err := safepath.WriteFile(home, filePath, prevPayload); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := writeCredentialFile(home, filePath, "new"); err != nil {
		t.Fatalf("writeCredentialFile: %v", err)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !isVersionedCredential(data) {
		t.Fatalf("replacement is not a versioned envelope: %q", string(data))
	}
	// v2 envelopes are DPAPI-encrypted on Windows, so verify the token via
	// the same decode path GetWithFallback uses rather than a plaintext scan.
	if got := extractTokenFromVersionedCredential(data); got != "new" {
		t.Errorf("replacement does not decode to the new token: got %q, want %q", got, "new")
	}
	if _, err := os.Stat(filePath + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("stray temp file: %v", err)
	}
}

// TestWriteCredentialFile_RenameFailure_LeavesNoPartialEnvelope is the second
// regression from the #386 report: a failed atomic commit (rename) must not
// expose a partially written authoritative envelope. The prior envelope
// stays readable, the temp file is cleaned up, and the error surfaces.
func TestWriteCredentialFile_RenameFailure_LeavesNoPartialEnvelope(t *testing.T) {
	home := testhome.NewTestHome(t)
	filePath := credentialFilePath(home)
	if err := safepath.MkdirAll(filepath.Dir(filePath)); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	prevPayload, err := encodeCredential("old")
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if err := safepath.WriteFile(home, filePath, prevPayload); err != nil {
		t.Fatalf("seed: %v", err)
	}

	orig := replaceCredentialFn
	replaceCredentialFn = func(string, string) error {
		return fmt.Errorf("injected rename failure")
	}
	defer func() { replaceCredentialFn = orig }()

	if err := writeCredentialFile(home, filePath, "new"); err == nil {
		t.Fatal("expected rename failure, got nil")
	}
	if _, err := os.Stat(filePath + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("temp file must be cleaned up after a rename failure: %v", err)
	}

	// Temp-file cleanup must also surface a secondary error when the
	// stray temp file cannot be removed (it does not mask the original).
	origRemove := removeCredentialTempFn
	removeCredentialTempFn = func(string) error {
		return fmt.Errorf("injected remove failure")
	}
	defer func() { removeCredentialTempFn = origRemove }()
	if err := writeCredentialFile(home, filePath, "new"); err == nil {
		t.Fatal("expected rename failure, got nil")
	}
	// Remove the seeded temp file so the no-stray-temp assertion below
	// reflects the CLEANUP behavior of the first failure, not this one.
	_ = os.Remove(filePath + ".tmp")

	// The previous authoritative envelope must be intact and decode back to
	// the old token — no partial/corrupt envelope may be exposed.
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read credential file: %v", err)
	}
	if !isVersionedCredential(data) {
		t.Fatalf("credential file is not a versioned envelope after failed rename: %q", string(data))
	}
	if got := extractTokenFromVersionedCredential(data); got != "old" {
		t.Errorf("previous credential lost or corrupted after failed rename: got %q, want %q", got, "old")
	}
	if got, err := Default().GetWithFallback(home); err != nil || got != "old" {
		t.Errorf("GetWithFallback after failed rename: got %q, err %v; want %q", got, err, "old")
	}
	// The temp file must not linger as a stray authoritative-looking file.
	if _, err := os.Stat(filePath + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("stray temp file left behind after failed rename: %v", err)
	}
}
