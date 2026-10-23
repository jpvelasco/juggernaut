package provider

import (
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
	"github.com/jpvelasco/juggernaut/v5/internal/schema"
	"github.com/jpvelasco/juggernaut/v5/internal/testutil"
)

// TestFromSchemaOptions_CopiesProfile pins the profile field copy: non-Claude
// providers get the named AWS profile through Options.Profile, exactly like
// AuthMode and Region.
func TestFromSchemaOptions_CopiesProfile(t *testing.T) {
	var opts Options
	opts.FromSchemaOptions(schema.Options{
		AuthMode: authmode.IAM,
		Region:   "us-west-2",
		Version:  "6.4.0",
		Scope:    "user",
		Profile:  "developer",
	})
	if opts.Profile != "developer" {
		t.Errorf("Profile = %q, want developer", opts.Profile)
	}
}

// TestJuggernautAuthBlock_Profile: the shared [juggernaut] table must carry the
// profile under auth.profile when set (so launch can export AWS_PROFILE) and
// omit the key entirely when empty, so existing configs render unchanged.
func TestJuggernautAuthBlock_Profile(t *testing.T) {
	opts := Options{
		AuthMode: authmode.IAM,
		Region:   "us-west-2",
		Version:  "6.4.0",
		Scope:    "user",
		Profile:  "developer",
	}
	blockMap, err := juggernautAuthBlock(opts, "us-west-2")
	if err != nil {
		t.Fatalf("juggernautAuthBlock: %v", err)
	}
	got, ok := testutil.NestedMapChain(blockMap, "auth", "profile")
	if !ok || got != "developer" {
		t.Errorf("auth.profile = %v, want developer", got)
	}

	opts.Profile = ""
	blockMap, err = juggernautAuthBlock(opts, "us-west-2")
	if err != nil {
		t.Fatalf("juggernautAuthBlock: %v", err)
	}
	if _, ok := testutil.NestedMapChain(blockMap, "auth", "profile"); ok {
		t.Error("auth.profile key must be omitted when no profile is set")
	}
}

// TestGrokAuthProviderCommand_WithProfile pins the named profile inside the
// auth_provider_command (so Grok's minted key uses it) and rejects profile
// names that are not shell-safe — the command runs through `sh -c`, exactly
// like the region it is embedded with.
func TestGrokAuthProviderCommand_WithProfile(t *testing.T) {
	got, err := grokAuthProviderCommand(authmode.IAM, "us-west-2", "developer")
	if err != nil {
		t.Fatalf("grokAuthProviderCommand: %v", err)
	}
	want := grokAuthCommand + " --iam --region=us-west-2 --profile=developer"
	if got != want {
		t.Errorf("IAM command = %q, want %q", got, want)
	}
	// No profile keeps the historical form: existing configs must not change.
	got, err = grokAuthProviderCommand(authmode.IAM, "us-west-2", "")
	if err != nil || got != grokAuthCommand+" --iam --region=us-west-2" {
		t.Errorf("IAM command without profile = %q, %v; want the bare region form", got, err)
	}
	// API-key mode never embeds a profile.
	got, err = grokAuthProviderCommand(authmode.BedrockAPIKey, "us-west-2", "developer")
	if err != nil {
		t.Fatalf("grokAuthProviderCommand: %v", err)
	}
	if got != grokAuthCommand {
		t.Errorf("API-key command with profile = %q, want %q", got, grokAuthCommand)
	}
	// Profile names are embedded in sh -c: anything outside the ASCII-safe
	// grammar must be rejected.
	for _, bad := range []string{"a b", "a;b", "a'b", "a\"b", "a$b", "a`b", "a\\b"} {
		if _, err := grokAuthProviderCommand(authmode.IAM, "us-west-2", bad); err == nil {
			t.Errorf("IAM command accepted unsafe profile name %q", bad)
		}
	}
	// Grammar matches grokRegionRE's ASCII-safe class, plus dots and
	// underscores that AWS SSO session and named profile forms may contain.
	for _, good := range []string{"developer", "prod-us", "team.1", "x_1"} {
		if _, err := grokAuthProviderCommand(authmode.IAM, "us-west-2", good); err != nil {
			t.Errorf("profile %q should be shell-safe: %v", good, err)
		}
	}
}
