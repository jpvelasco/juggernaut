// sidecar.go — auth-mode metadata sidecar for providers whose vendor-validated
// config file rejects unknown top-level keys.
//
// OpenCode validates opencode.json against a strict schema
// (https://opencode.ai/config.json, Config.additionalProperties: false), so the
// juggernaut auth-mode block cannot live inside the vendor file. Instead, a
// provider that implements SidecarAuthSource stores the same block in an
// owner-only .juggernaut.json next to the config. The sidecar is non-secret
// (auth mode, region, version metadata) — bearer tokens stay in the keychain.
package provider

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jpvelasco/juggernaut/v5/internal/config"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

// SidecarFilename is the sidecar file name written next to a provider config.
const SidecarFilename = ".juggernaut.json"

// SidecarAuthSource is an optional extension implemented by providers that keep
// the juggernaut auth-mode block outside their vendor-validated config file.
// Providers without the extension (Claude, Codex, Grok) store the block in the
// config itself.
type SidecarAuthSource interface {
	// SidecarPath returns the sidecar path for the given scope: the file next
	// to ConfigPath(home, scope).
	SidecarPath(home, scope string) (string, error)

	// SidecarPaths returns the sidecar paths in launch precedence order
	// (project then user) so launch reads project-then-user like the config.
	SidecarPaths(home string) []string
}

// HasSidecar reports whether the provider keeps its auth-mode block in a
// sidecar file. It is a type assertion so cmd/ never branches on CLI name.
func HasSidecar(p Provider) bool {
	_, ok := p.(SidecarAuthSource)
	return ok
}

// writeSidecarFile persists the sidecar with owner-only permissions. User-scope
// paths are home-anchored (safepath containment); project paths sit in the
// working tree, so they are written directly (still 0o600).
func writeSidecarFile(path string, data []byte) error {
	if home, err := safepath.HomeDir(); err == nil && filepath.IsAbs(path) {
		return safepath.WriteFile(home, path, data)
	}
	if err := safepath.MkdirAll(filepath.Dir(path)); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// WriteSidecar persists the current juggernaut auth-mode block to the
// provider's sidecar for opts.Scope. It is a no-op for providers without the
// sidecar extension.
func WriteSidecar(p Provider, home string, opts Options) error {
	src, ok := p.(SidecarAuthSource)
	if !ok {
		return nil
	}
	path, err := src.SidecarPath(home, opts.Scope)
	if err != nil {
		return err
	}
	block, err := juggernautAuthBlock(opts, opts.Region)
	if err != nil {
		return err
	}
	// Wrap under "juggernaut" so config.ParseJuggernautBlock (with its
	// managedBy gate) parses sidecars and in-file blocks identically.
	doc := map[string]any{"juggernaut": block}
	encoded, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling %s: %w", path, err)
	}
	return writeSidecarFile(path, append(encoded, '\n'))
}

// ReadSidecarAuthMode returns the first non-empty juggernaut.auth.mode across
// the provider's sidecar paths (project then user). ok is false when the
// provider has no sidecar or no readable sidecar declares a mode.
func ReadSidecarAuthMode(p Provider, home string) (mode string, ok bool) {
	src, isSidecar := p.(SidecarAuthSource)
	if !isSidecar {
		return "", false
	}
	for _, path := range src.SidecarPaths(home) {
		block, ok := readSidecarBlock(path)
		if !ok {
			continue
		}
		if mode := BlockAuthMode(block); mode != "" {
			return mode, true
		}
	}
	return "", false
}

// BlockAuthMode extracts the auth mode from a parsed juggernaut block
// ("" when absent).
func BlockAuthMode(block map[string]any) string {
	auth, ok := block["auth"].(map[string]any)
	if !ok {
		return ""
	}
	mode, _ := auth["mode"].(string)
	return mode
}

// SidecarExists reports whether the sidecar for scope exists on disk.
func SidecarExists(p Provider, home, scope string) bool {
	src, ok := p.(SidecarAuthSource)
	if !ok {
		return false
	}
	path, err := src.SidecarPath(home, scope)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// RemoveSidecar deletes the provider's sidecar files for the given scopes.
// Missing files are not an error. No-op for non-sidecar providers.
func RemoveSidecar(p Provider, home string, scopes []string) error {
	src, ok := p.(SidecarAuthSource)
	if !ok {
		return nil
	}
	for _, scope := range scopes {
		path, err := src.SidecarPath(home, scope)
		if err != nil {
			return err
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// MigrateSidecarLegacy migrates a sidecar provider's existing config in place
// (called under the config file lock on a plain apply): it removes the
// in-file juggernaut block left by v6.2.0–v6.3.0 OpenCode applies (now stored
// in the sidecar) and drops a stale "whitelist" leaf under
// provider.amazon-bedrock when it is absent/empty (a pre-fix nil slice
// marshals back to null, which OpenCode's schema rejects). A user-owned
// "juggernaut" key (different managedBy, or no managedBy) is preserved. It
// returns a migration notice when it changed anything; "" otherwise. No-op for
// providers without the sidecar extension.
func MigrateSidecarLegacy(p Provider, existing map[string]any) string {
	if _, ok := p.(SidecarAuthSource); !ok {
		return ""
	}
	if existing == nil {
		return ""
	}
	var notices []string
	if _, ok := config.ParseJuggernautBlock(existing); ok {
		delete(existing, "juggernaut")
		notices = append(notices, "removed the legacy in-file juggernaut block (auth metadata now lives in the sidecar file)")
	}
	if sub, stale := staleOwnedSubKeyLeaf(existing, "provider", "amazon-bedrock", "whitelist"); stale {
		delete(sub, "whitelist")
		notices = append(notices, "dropped the stale empty whitelist (OpenCode rejects null/empty arrays)")
	}
	if len(notices) == 0 {
		return ""
	}
	return strings.Join(notices, "; ")
}

// staleOwnedSubKeyLeaf navigates table[subTable][leaf] and reports whether the
// leaf is a null/empty array (the JSON shape pre-fix applies can leave
// behind). table is the top-level key, subTable its nested key.
func staleOwnedSubKeyLeaf(existing map[string]any, table, subTable, leaf string) (map[string]any, bool) {
	tbl, ok := existing[table].(map[string]any)
	if !ok {
		return nil, false
	}
	sub, ok := tbl[subTable].(map[string]any)
	if !ok {
		return nil, false
	}
	leafVal, present := sub[leaf]
	if !present {
		return nil, false
	}
	if arr, ok := leafVal.([]any); ok {
		return sub, len(arr) == 0
	}
	return sub, leafVal == nil
}

// ReadSidecarFile reads one sidecar file and returns its parsed juggernaut
// block, or nil when the file is missing, unreadable, malformed, or not a
// Juggernaut block. The managedBy gate in config.ParseJuggernautBlock means a
// user-owned file with the same name is never misread.
func ReadSidecarFile(path string) map[string]any {
	block, _ := readSidecarBlock(path)
	return block
}

// readSidecarBlock reads and parses one sidecar file, returning its juggernaut
// block when present. The path is provider-derived, not user input; a
// missing/unreadable/malformed file is a normal "absent" outcome.
func readSidecarBlock(path string) (map[string]any, bool) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is provider-derived, not user input
	if err != nil {
		return nil, false
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, false
	}
	if _, ok := config.ParseJuggernautBlock(doc); !ok {
		return nil, false
	}
	block, _ := doc["juggernaut"].(map[string]any)
	return block, true
}
