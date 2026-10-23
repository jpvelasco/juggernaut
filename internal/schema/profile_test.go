package schema_test

import (
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
	"github.com/jpvelasco/juggernaut/v5/internal/schema"
)

// TestBuild_ProfilePersists checks that a named AWS profile set via apply
// --profile lands in the juggernaut.auth block (so launch/doctor can read it
// back) AND in the injected environment (AWS_PROFILE), so the launched CLI and
// its credential chain use the profile deterministically.
func TestBuild_ProfilePersists(t *testing.T) {
	cfg := testConfig()
	opts := schema.Options{
		AuthMode:      authmode.IAM,
		Region:        "us-west-2",
		Effort:        "high",
		Use1M:         true,
		AuthValidated: true,
		Profile:       "developer",
	}
	block, err := schema.Build(cfg, opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if block.Auth.Profile != "developer" {
		t.Errorf("auth.profile = %q, want developer", block.Auth.Profile)
	}
	if got := block.Env["AWS_PROFILE"]; got != "developer" {
		t.Errorf("env AWS_PROFILE = %q, want developer", got)
	}
}

// TestBuild_ProfileOmittedWhenEmpty checks the default path stays byte-stable:
// with no --profile, both the block key and the env var must be absent —
// existing configs written by earlier versions must keep rendering identically.
func TestBuild_ProfileOmittedWhenEmpty(t *testing.T) {
	cfg := testConfig()
	opts := schema.Options{
		AuthMode:      authmode.IAM,
		Region:        "us-west-2",
		Effort:        "high",
		Use1M:         true,
		AuthValidated: true,
	}
	block, err := schema.Build(cfg, opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if block.Auth.Profile != "" {
		t.Errorf("auth.profile = %q, want empty", block.Auth.Profile)
	}
	if _, ok := block.Env["AWS_PROFILE"]; ok {
		t.Error("AWS_PROFILE env must not be set when no profile was requested")
	}
}

// TestBuild_ProfileOnlyWithIAMMode rejects building a block that records a
// profile under non-IAM auth: bearer-token auth never reads a profile, so
// persisting one would imply a launch-time override that cannot matter.
func TestBuild_ProfileOnlyWithIAMMode(t *testing.T) {
	cfg := testConfig()
	opts := schema.Options{
		AuthMode:      authmode.BedrockAPIKey,
		Region:        "us-west-2",
		Effort:        "high",
		Use1M:         true,
		AuthValidated: true,
		Profile:       "developer",
	}
	_, err := schema.Build(cfg, opts)
	if err == nil {
		t.Fatal("Build() accepted a profile with non-IAM auth")
	}
	if !strings.Contains(err.Error(), "--profile") {
		t.Errorf("error = %v, want a --profile message", err)
	}
}
