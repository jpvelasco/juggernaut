package keychain

import (
	"path/filepath"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

func TestWriteCredentialFileFailsWhenExistingPathCannotBeRemoved(t *testing.T) {
	home := t.TempDir()
	filePath := credentialFilePath(home)

	// Make the credential path a non-empty directory so removal fails on all
	// supported platforms.
	blockerPath := filepath.Join(filePath, "blocker")
	if err := safepath.WriteFile(home, blockerPath, []byte("blocker")); err != nil {
		t.Fatalf("creating blocker: %v", err)
	}

	if err := writeCredentialFile(home, filePath, "secret"); err == nil {
		t.Fatal("expected an error when the existing credential path cannot be removed")
	}
}
