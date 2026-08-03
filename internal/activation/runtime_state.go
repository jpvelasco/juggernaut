package activation

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

const runtimeStateVersion = 1

// RuntimeState is the non-secret launch configuration retained outside a
// vendor-owned config file. It lets activation survive that file being reset.
type RuntimeState struct {
	AuthMode string            `json:"authMode"`
	Env      map[string]string `json:"env,omitempty"`
}

type runtimeStateFile struct {
	SchemaVersion int    `json:"schemaVersion"`
	ManagedBy     string `json:"managedBy"`
	CLI           string `json:"cli"`
	RuntimeState
}

// RuntimeStatePath returns the owner-local fallback state path for a CLI.
func RuntimeStatePath(home, cli string) (string, error) {
	if err := validateCLIName(cli); err != nil {
		return "", err
	}
	return safepath.JoinUnder(home, ".juggernaut", "runtime", cli+".json")
}

// SaveRuntimeState atomically stores non-secret user-scope launch state.
func SaveRuntimeState(home, cli string, state RuntimeState) error {
	if err := validateRuntimeState(state); err != nil {
		return err
	}
	path, err := RuntimeStatePath(home, cli)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(runtimeStateFile{
		SchemaVersion: runtimeStateVersion,
		ManagedBy:     "juggernaut",
		CLI:           cli,
		RuntimeState:  state,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding runtime state: %w", err)
	}

	tmp := path + ".tmp"
	if err := safepath.WriteFile(home, tmp, append(data, '\n')); err != nil {
		return fmt.Errorf("writing runtime state: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("committing runtime state: %w", err)
	}
	return nil
}

// LoadRuntimeState reads a CLI's fallback state. A missing file is not an error.
func LoadRuntimeState(home, cli string) (RuntimeState, bool, error) {
	path, err := RuntimeStatePath(home, cli)
	if err != nil {
		return RuntimeState{}, false, err
	}
	data, err := safepath.ReadFile(home, path)
	if errors.Is(err, os.ErrNotExist) {
		return RuntimeState{}, false, nil
	}
	if err != nil {
		return RuntimeState{}, false, fmt.Errorf("reading runtime state: %w", err)
	}

	var stored runtimeStateFile
	if err := json.Unmarshal(data, &stored); err != nil {
		return RuntimeState{}, false, fmt.Errorf("parsing %s: %w", filepath.Base(path), err)
	}
	if stored.SchemaVersion != runtimeStateVersion {
		return RuntimeState{}, false, fmt.Errorf(
			"unsupported runtime state version %d (expected %d)",
			stored.SchemaVersion, runtimeStateVersion)
	}
	if stored.ManagedBy != "juggernaut" || stored.CLI != cli {
		return RuntimeState{}, false, fmt.Errorf("runtime state ownership does not match CLI %q", cli)
	}
	if err := validateRuntimeState(stored.RuntimeState); err != nil {
		return RuntimeState{}, false, err
	}
	return stored.RuntimeState, true, nil
}

// RemoveRuntimeState removes a CLI's fallback state.
func RemoveRuntimeState(home, cli string) error {
	path, err := RuntimeStatePath(home, cli)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing runtime state: %w", err)
	}
	return nil
}

func validateRuntimeState(state RuntimeState) error {
	if state.AuthMode != authmode.IAM && !authmode.IsBedrockAPIKey(state.AuthMode) {
		return fmt.Errorf("invalid runtime auth mode %q", state.AuthMode)
	}
	for key, value := range state.Env {
		if key == "" || strings.ContainsAny(key, "=\x00") {
			return fmt.Errorf("invalid runtime environment name %q", key)
		}
		if strings.ContainsRune(value, '\x00') {
			return fmt.Errorf("runtime environment %q contains a null byte", key)
		}
		if sensitiveRuntimeEnvKey(key) {
			return fmt.Errorf("runtime state must not contain credential environment %s", key)
		}
	}
	return nil
}

func sensitiveRuntimeEnvKey(key string) bool {
	key = strings.ToUpper(key)
	if key == authmode.BedrockAuthEnvName {
		return true
	}
	for _, marker := range []string{
		"_API_KEY",
		"_AUTH_TOKEN",
		"_ACCESS_KEY",
		"_ACCESS_TOKEN",
		"_BEARER_TOKEN",
		"_CREDENTIAL",
		"_PASSWORD",
		"_PRIVATE_KEY",
		"_SECRET",
		"_SESSION_TOKEN",
		"_TOKEN_",
	} {
		if strings.Contains(key, marker) {
			return true
		}
	}
	return strings.HasSuffix(key, "_TOKEN")
}
