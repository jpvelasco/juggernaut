package cmd

import (
	"context"
	"strings"
	"testing"
)

// TestAuthToken_Command_IAM_ProfileThreadsMinter: --profile reaches the
// minter, so the short-term key is minted from the named profile's
// credentials rather than the ambient default chain — that is what makes
// Grok's auth_provider_command work on profile-only machines.
func TestAuthToken_Command_IAM_ProfileThreadsMinter(t *testing.T) {
	var gotRegion, gotProfile string
	stubMintBedrockToken(t, func(_ context.Context, region, profile string) (string, error) {
		gotRegion = region
		gotProfile = profile
		return "iam-token", nil
	})

	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{"auth-token", "--iam", "--region=us-west-2", "--profile=developer"}); err != nil {
			t.Fatalf("auth-token --iam: %v", err)
		}
	})
	if gotProfile != "developer" {
		t.Errorf("minted with profile %q, want developer", gotProfile)
	}
	if gotRegion != "us-west-2" {
		t.Errorf("minted for region %q, want us-west-2", gotRegion)
	}
	if out == "" {
		t.Error("expected token output on stdout")
	}
}

// TestAuthToken_Command_ProfileWithoutIAM: --profile only changes the IAM
// mint path; on the keychain path it is a command error (a silent no-op
// would look like it did something).
func TestAuthToken_Command_ProfileWithoutIAM(t *testing.T) {
	out := captureStdout(t, func() {
		err := ExecuteArgs([]string{"auth-token", "--profile=developer"})
		if err == nil {
			t.Fatal("expected error for --profile without --iam")
		}
		if !strings.Contains(err.Error(), "--profile requires --iam") {
			t.Errorf("error = %v, want a --profile-requires-iam message", err)
		}
	})
	if out != "" {
		t.Errorf("expected no stdout on error, got %q", out)
	}
}

// TestAuthToken_Command_IAM_EmptyProfileKeepsAmbientChain: no --profile keeps
// the historical mint behavior — the ambient credential chain decides.
func TestAuthToken_Command_IAM_EmptyProfileKeepsAmbientChain(t *testing.T) {
	var gotProfile string
	stubMintBedrockToken(t, func(_ context.Context, _ string, profile string) (string, error) {
		gotProfile = profile
		return "iam-token", nil
	})

	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{"auth-token", "--iam", "--region=us-west-2"}); err != nil {
			t.Fatalf("auth-token --iam: %v", err)
		}
	})
	if gotProfile != "" {
		t.Errorf("minted with profile %q, want empty (ambient chain)", gotProfile)
	}
	if out == "" {
		t.Error("expected token output on stdout")
	}
}
