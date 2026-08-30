package cmd

import (
	"strings"
	"testing"
)

func TestTask1_MantleFlagsRemoved(t *testing.T) {
	for _, name := range []string{"mantle", "no-mantle", "mantle-url"} {
		if f := applyCmd.Flags().Lookup(name); f != nil {
			t.Errorf("flag --%s still registered; expected Mantle flags removed in Task 1 (BREAKING CHANGE v6)", name)
		}
	}
}

func TestTask1_MantleUnknownFlagError(t *testing.T) {
	_ = setupApplyTest(t)
	err := ExecuteArgs([]string{"apply", "--auth=iam", "--region=us-west-2", "--mantle"})
	if err == nil {
		t.Fatal("expected unknown flag error for --mantle after removal")
	}
	if !strings.Contains(err.Error(), "unknown flag") || !strings.Contains(err.Error(), "mantle") {
		t.Errorf("expected unknown flag error for --mantle, got %v", err)
	}
}

func TestTask1_MantleURLUnknownFlagError(t *testing.T) {
	_ = setupApplyTest(t)
	err := ExecuteArgs([]string{"apply", "--auth=iam", "--region=us-west-2", "--mantle-url=https://example.test"})
	if err == nil {
		t.Fatal("expected unknown flag error for --mantle-url after removal")
	}
	if !strings.Contains(err.Error(), "unknown flag") || !strings.Contains(err.Error(), "mantle-url") {
		t.Errorf("expected unknown flag error for --mantle-url, got %v", err)
	}
}

func TestTask1_NoMantleUnknownFlagError(t *testing.T) {
	_ = setupApplyTest(t)
	err := ExecuteArgs([]string{"apply", "--auth=iam", "--region=us-west-2", "--no-mantle"})
	if err == nil {
		t.Fatal("expected unknown flag error for --no-mantle after removal")
	}
	if !strings.Contains(err.Error(), "unknown flag") || !strings.Contains(err.Error(), "no-mantle") {
		t.Errorf("expected unknown flag error for --no-mantle, got %v", err)
	}
}
