package keychain

import (
	"os"
	"path/filepath"
	"strings"
)

// ProfileBackend stores the bearer token as a plaintext file under the user's
// config directory. It is cross-platform and v3-compatible: the path and format
// match the v3 PowerShell/Bash profile token store, so v3 tokens migrate for free.
//
// Layout: $JUGGERNAUT_PROFILE_TOKEN_PATH, else
// $XDG_CONFIG_HOME/juggernaut/bearer-token, else <home>/.config/juggernaut/bearer-token.
type ProfileBackend struct {
	home string
}

// NewProfileBackend creates a ProfileBackend rooted at the given home directory.
func NewProfileBackend(home string) *ProfileBackend {
	return &ProfileBackend{home: home}
}

func (b *ProfileBackend) path() string {
	if p := os.Getenv("JUGGERNAUT_PROFILE_TOKEN_PATH"); p != "" {
		return p
	}
	configRoot := os.Getenv("XDG_CONFIG_HOME")
	if configRoot == "" {
		configRoot = filepath.Join(b.home, ".config")
	}
	return filepath.Join(configRoot, "juggernaut", "bearer-token")
}

// Set writes the token as UTF-8 plaintext with owner-only permissions.
func (b *ProfileBackend) Set(token string) error {
	path := b.path()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(token), 0o600)
}

// Get returns the stored token trimmed of surrounding whitespace, or "" if absent.
func (b *ProfileBackend) Get() (string, error) {
	data, err := os.ReadFile(b.path())
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// Delete removes the token file. Silent if absent.
func (b *ProfileBackend) Delete() error {
	if err := os.Remove(b.path()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
