package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApply_DryRun_IAM(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	err := ExecuteArgs([]string{
		"apply",
		"--auth=iam",
		"--region=us-west-2",
		"--dry-run",
		"--skip-preflight",
	})
	if err != nil {
		t.Fatalf("apply --dry-run error: %v", err)
	}

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	if _, err := os.Stat(settingsPath); !os.IsNotExist(err) {
		t.Error("dry-run should not create settings.json")
	}
}

func TestApply_DryRun_BedrockAPIKey(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	err := ExecuteArgs([]string{
		"apply",
		"--auth=bedrock-api-key",
		"--bedrock-key=test-key-value",
		"--region=us-west-2",
		"--dry-run",
		"--skip-preflight",
	})
	if err != nil {
		t.Fatalf("apply --dry-run bedrock-api-key error: %v", err)
	}

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	if _, err := os.Stat(settingsPath); !os.IsNotExist(err) {
		t.Error("dry-run should not create settings.json")
	}
}
