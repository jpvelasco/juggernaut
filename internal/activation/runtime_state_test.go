package activation

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
	"github.com/jpvelasco/juggernaut/v5/internal/testutil"
)

func TestRuntimeStateRoundTripAndRemoval(t *testing.T) {
	home := testutil.NewTestHome(t)
	want := RuntimeState{
		AuthMode: authmode.IAM,
		Env: map[string]string{
			"CLAUDE_CODE_USE_BEDROCK": "1",
			"AWS_REGION":              "us-west-2",
		},
	}

	if err := SaveRuntimeState(home, "claude", want); err != nil {
		t.Fatalf("SaveRuntimeState: %v", err)
	}
	got, found, err := LoadRuntimeState(home, "claude")
	if err != nil || !found {
		t.Fatalf("LoadRuntimeState = found %v, err %v", found, err)
	}
	if got.AuthMode != want.AuthMode || got.Env["AWS_REGION"] != "us-west-2" {
		t.Errorf("runtime state = %+v, want %+v", got, want)
	}

	path, err := RuntimeStatePath(home, "claude")
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		info, statErr := os.Stat(path)
		if statErr != nil {
			t.Fatal(statErr)
		}
		if gotPerm := info.Mode().Perm() & 0o077; gotPerm != 0 {
			t.Errorf("runtime state permissions expose user data: %o", info.Mode().Perm())
		}
	}

	if err := RemoveRuntimeState(home, "claude"); err != nil {
		t.Fatalf("RemoveRuntimeState: %v", err)
	}
	if _, found, err := LoadRuntimeState(home, "claude"); err != nil || found {
		t.Fatalf("removed state = found %v, err %v; want false, nil", found, err)
	}
	if err := RemoveRuntimeState(home, "claude"); err != nil {
		t.Fatalf("second RemoveRuntimeState should be idempotent: %v", err)
	}
}

func TestRuntimeStateRejectsInvalidOrSecretData(t *testing.T) {
	home := testutil.NewTestHome(t)
	tests := []struct {
		name  string
		cli   string
		state RuntimeState
	}{
		{
			name:  "invalid CLI",
			cli:   "../claude",
			state: RuntimeState{AuthMode: authmode.IAM},
		},
		{
			name:  "invalid auth",
			cli:   "claude",
			state: RuntimeState{AuthMode: "unknown"},
		},
		{
			name: "bearer token",
			cli:  "claude",
			state: RuntimeState{
				AuthMode: authmode.BedrockAPIKey,
				Env:      map[string]string{strings.ToLower(authmode.BedrockAuthEnvName): "secret"},
			},
		},
		{
			name: "AWS session token",
			cli:  "claude",
			state: RuntimeState{
				AuthMode: authmode.IAM,
				Env:      map[string]string{"AWS_SESSION_TOKEN": "secret"},
			},
		},
		{
			name: "Anthropic API key",
			cli:  "claude",
			state: RuntimeState{
				AuthMode: authmode.BedrockAPIKey,
				Env:      map[string]string{"ANTHROPIC_API_KEY": "secret"},
			},
		},
		{
			name: "invalid environment name",
			cli:  "claude",
			state: RuntimeState{
				AuthMode: authmode.IAM,
				Env:      map[string]string{"BAD=NAME": "value"},
			},
		},
		{
			name: "invalid environment value",
			cli:  "claude",
			state: RuntimeState{
				AuthMode: authmode.IAM,
				Env:      map[string]string{"AWS_REGION": "us-west-2\x00bad"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := SaveRuntimeState(home, tt.cli, tt.state); err == nil {
				t.Fatal("SaveRuntimeState should reject invalid state")
			}
		})
	}
}

func TestLoadRuntimeStateRejectsMalformedAndMismatchedFiles(t *testing.T) {
	home := testutil.NewTestHome(t)
	path, err := RuntimeStatePath(home, "claude")
	if err != nil {
		t.Fatal(err)
	}
	if err := safepath.MkdirAll(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}

	for _, data := range []string{
		`{`,
		`{"schemaVersion":99,"managedBy":"juggernaut","cli":"claude","authMode":"iam"}`,
		`{"schemaVersion":1,"managedBy":"other","cli":"claude","authMode":"iam"}`,
	} {
		if err := safepath.WriteFile(home, path, []byte(data)); err != nil {
			t.Fatal(err)
		}
		if _, _, err := LoadRuntimeState(home, "claude"); err == nil {
			t.Fatalf("LoadRuntimeState should reject %s", data)
		}
	}
}
