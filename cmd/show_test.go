package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestShow_Text_AfterApply(t *testing.T) {
	_ = setupApplyTest(t)

	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply error: %v", err)
	}

	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{"show", "--scope=user"}); err != nil {
			t.Fatalf("show error: %v", err)
		}
	})

	if !strings.Contains(out, "user scope") {
		t.Errorf("show should print the user scope header, got:\n%s", out)
	}
	if !strings.Contains(out, "managedBy") {
		t.Errorf("show should print the juggernaut block, got:\n%s", out)
	}
}

func TestShow_JSON_AfterApply(t *testing.T) {
	_ = setupApplyTest(t)

	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply error: %v", err)
	}

	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{"show", "--scope=user", "--json"}); err != nil {
			t.Fatalf("show --json error: %v", err)
		}
	})

	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("show --json should emit valid JSON, got:\n%s\nerr: %v", out, err)
	}
	if _, ok := parsed["user"]; !ok {
		t.Errorf("show --json should contain the user scope key, got:\n%s", out)
	}
}

func TestShow_NotConfigured(t *testing.T) {
	_ = setupApplyTest(t)

	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{"show", "--scope=user"}); err != nil {
			t.Fatalf("show on clean home error: %v", err)
		}
	})

	if !strings.Contains(out, "not configured") {
		t.Errorf("show on a clean home should report 'not configured', got:\n%s", out)
	}
}
