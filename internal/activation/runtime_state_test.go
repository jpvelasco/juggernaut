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
		`{"schemaVersion":1,"managedBy":"juggernaut","cli":"claude","authMode":"unknown"}`,
	} {
		if err := safepath.WriteFile(home, path, []byte(data)); err != nil {
			t.Fatal(err)
		}
		if _, _, err := LoadRuntimeState(home, "claude"); err == nil {
			t.Fatalf("LoadRuntimeState should reject %s", data)
		}
	}
}

func TestRuntimeStateOperationsRejectInvalidCLI(t *testing.T) {
	home := testutil.NewTestHome(t)

	if _, found, err := LoadRuntimeState(home, "../claude"); err == nil || found {
		t.Fatalf("LoadRuntimeState invalid CLI = found %v, err %v; want error", found, err)
	}
	if err := RemoveRuntimeState(home, "../claude"); err == nil {
		t.Fatal("RemoveRuntimeState should reject an invalid CLI")
	}
}

func TestSaveRuntimeStateReportsFilesystemFailures(t *testing.T) {
	state := RuntimeState{AuthMode: authmode.IAM}

	t.Run("write", func(t *testing.T) {
		home := testutil.NewTestHome(t)
		if err := os.WriteFile(filepath.Join(home, ".juggernaut"), []byte("not a directory"), 0o600); err != nil {
			t.Fatal(err)
		}

		err := SaveRuntimeState(home, "claude", state)
		if err == nil || !strings.Contains(err.Error(), "writing runtime state") {
			t.Fatalf("SaveRuntimeState write error = %v", err)
		}
	})

	t.Run("rename cleans temporary file", func(t *testing.T) {
		home := testutil.NewTestHome(t)
		path, err := RuntimeStatePath(home, "claude")
		if err != nil {
			t.Fatal(err)
		}
		if err := safepath.MkdirAll(path); err != nil {
			t.Fatal(err)
		}

		err = SaveRuntimeState(home, "claude", state)
		if err == nil || !strings.Contains(err.Error(), "committing runtime state") {
			t.Fatalf("SaveRuntimeState rename error = %v", err)
		}
		if _, statErr := os.Stat(path + ".tmp"); !os.IsNotExist(statErr) {
			t.Fatalf("temporary runtime state was not cleaned up: %v", statErr)
		}
	})
}

func TestLoadRuntimeStateReportsReadFailure(t *testing.T) {
	home := testutil.NewTestHome(t)
	path, err := RuntimeStatePath(home, "claude")
	if err != nil {
		t.Fatal(err)
	}
	if err := safepath.MkdirAll(path); err != nil {
		t.Fatal(err)
	}

	if _, found, err := LoadRuntimeState(home, "claude"); err == nil || found ||
		!strings.Contains(err.Error(), "reading runtime state") {
		t.Fatalf("LoadRuntimeState directory = found %v, err %v", found, err)
	}
}

func TestRemoveRuntimeStateReportsFilesystemFailure(t *testing.T) {
	home := testutil.NewTestHome(t)
	path, err := RuntimeStatePath(home, "claude")
	if err != nil {
		t.Fatal(err)
	}
	if err := safepath.MkdirAll(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "keep"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	err = RemoveRuntimeState(home, "claude")
	if err == nil || !strings.Contains(err.Error(), "removing runtime state") {
		t.Fatalf("RemoveRuntimeState error = %v", err)
	}
}
