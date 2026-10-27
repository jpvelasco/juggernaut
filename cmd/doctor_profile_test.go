package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
	"github.com/jpvelasco/juggernaut/v5/internal/doctor"
	"github.com/jpvelasco/juggernaut/v5/internal/provider"
	"github.com/jpvelasco/juggernaut/v5/internal/testutil"
)

// writeManagedClaudeSettings seeds a user-scope settings.json whose juggernaut
// block carries the given IAM auth fields (profile may be empty).
func writeManagedClaudeSettings(t *testing.T, home, profile string) {
	t.Helper()
	dir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(dir, 0o700); err != nil { // nosemgrep: go.lang.correctness.permissions.file_permission.incorrect-default-permission -- directory
		t.Fatalf("mkdir .claude: %v", err)
	}
	auth := `{"mode": "iam", "region": "us-west-2"`
	if profile != "" {
		auth += `, "profile": "` + profile + `"`
	}
	auth += `}`
	settings := `{
  "juggernaut": {
    "auth": ` + auth + `,
    "meta": {"managedBy": "juggernaut"}
  }
}`
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(settings), 0o600); err != nil {
		t.Fatalf("write settings.json: %v", err)
	}
}

// TestCheckAWSProfile_StoredProfile: IAM config with a stored named profile
// resolves the profile and reports it OK. The stored name is re-validated so a
// renamed/removed profile is surfaced instead of silently breaking at runtime.
func TestCheckAWSProfile_StoredProfile(t *testing.T) {
	home := testutil.NewTestHome(t)
	writeManagedClaudeSettings(t, home, "developer")
	stubValidateProfileOK(t)
	r := doctor.NewReport()
	checkAWSProfile(r, provider.MustGet("claude"), home, []string{"user"})

	if r.HasWarnings() || r.HasFailures() {
		t.Errorf("stored profile should be OK, report:\n%s", r.String())
	}
	if !strings.Contains(r.String(), "developer") {
		t.Errorf("report should name the profile, got:\n%s", r.String())
	}
}

// TestCheckAWSProfile_EnvProfile: without a stored profile, an ambient
// AWS_PROFILE still resolves credentials deterministically and is OK.
func TestCheckAWSProfile_EnvProfile(t *testing.T) {
	home := testutil.NewTestHome(t)
	writeManagedClaudeSettings(t, home, "")
	t.Setenv("AWS_PROFILE", "from-env")
	stubValidateProfileOK(t)
	r := doctor.NewReport()
	checkAWSProfile(r, provider.MustGet("claude"), home, []string{"user"})

	if r.HasWarnings() || r.HasFailures() {
		t.Errorf("ambient AWS_PROFILE should be OK, report:\n%s", r.String())
	}
	if !strings.Contains(r.String(), "from-env") {
		t.Errorf("report should name the env profile, got:\n%s", r.String())
	}
}

// TestCheckAWSProfile_NoProfileWarns: IAM mode with no profile anywhere is the
// exact condition from issue #510 — doctor used to stay silent while the CLI
// runtime failed with NoCredentials. Warn with the two actionable fixes.
func TestCheckAWSProfile_NoProfileWarns(t *testing.T) {
	home := testutil.NewTestHome(t)
	writeManagedClaudeSettings(t, home, "")
	t.Setenv("AWS_PROFILE", "")
	stubValidateProfileOK(t)
	r := doctor.NewReport()
	checkAWSProfile(r, provider.MustGet("claude"), home, []string{"user"})

	if !strings.Contains(r.String(), "aws profile") {
		t.Errorf("expected an aws profile check, got:\n%s", r.String())
	}
	if !r.HasWarnings() {
		t.Errorf("no-profile IAM should warn, report:\n%s", r.String())
	}
	detail := r.String()
	if !strings.Contains(detail, "`juggernaut apply --profile") ||
		!strings.Contains(detail, "AWS_PROFILE") {
		t.Errorf("warning should give both fixes (apply --profile / export AWS_PROFILE), got:\n%s", detail)
	}
}

// TestCheckAWSProfile_StoredProfileUnusable: a stored profile that can no
// longer resolve (renamed, deleted from shared config) must warn, not OK.
func TestCheckAWSProfile_StoredProfileUnusable(t *testing.T) {
	home := testutil.NewTestHome(t)
	writeManagedClaudeSettings(t, home, "developer")
	stubValidateProfileErr(t, errors.New("profile 'developer' not found in the shared config files"))
	r := doctor.NewReport()
	checkAWSProfile(r, provider.MustGet("claude"), home, []string{"user"})

	if !r.HasWarnings() {
		t.Errorf("unusable stored profile should warn, report:\n%s", r.String())
	}
	if !strings.Contains(r.String(), "developer") {
		t.Errorf("warning should name the broken profile, got:\n%s", r.String())
	}
}

// TestCheckAWSProfile_BearerAuthSkipped: bearer-token auth never reads an AWS
// profile — no check, no warning.
func TestCheckAWSProfile_BearerAuthSkipped(t *testing.T) {
	home := testutil.NewTestHome(t)
	writeManagedClaudeSettings(t, home, "")
	if err := os.WriteFile(filepath.Join(home, ".claude", "settings.json"),
		[]byte(`{
  "juggernaut": {
    "auth": {"mode": "`+authmode.BedrockAPIKey+`", "region": "us-west-2"},
    "meta": {"managedBy": "juggernaut"}
  }
}`), 0o600); err != nil {
		t.Fatalf("write settings.json: %v", err)
	}
	t.Setenv("AWS_PROFILE", "")
	r := doctor.NewReport()
	checkAWSProfile(r, provider.MustGet("claude"), home, []string{"user"})

	if strings.Contains(r.String(), "aws profile") {
		t.Errorf("bearer auth must not report an aws profile check, got:\n%s", r.String())
	}
}

// TestCheckAWSProfile_UnmanagedSkipped: a config Juggernaut doesn't own has no
// juggernaut block to read; the check must not fire on foreign configs.
func TestCheckAWSProfile_UnmanagedSkipped(t *testing.T) {
	home := testutil.NewTestHome(t)
	dir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(dir, 0o700); err != nil { // nosemgrep: go.lang.correctness.permissions.file_permission.incorrect-default-permission -- directory
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"env": {}}`), 0o600); err != nil {
		t.Fatalf("write settings.json: %v", err)
	}
	t.Setenv("AWS_PROFILE", "")
	r := doctor.NewReport()
	checkAWSProfile(r, provider.MustGet("claude"), home, []string{"user"})

	if strings.Contains(r.String(), "aws profile") {
		t.Errorf("unmanaged config must not report an aws profile check, got:\n%s", r.String())
	}
}

// TestCheckAWSProfile_SidecarProvider: OpenCode stores the block in the
// sidecar, not opencode.json — the check must read the sidecar.
func TestCheckAWSProfile_SidecarProvider(t *testing.T) {
	home := testutil.NewTestHome(t)
	if err := os.MkdirAll(filepath.Join(home, ".config", "opencode"), 0o700); err != nil { // nosemgrep: go.lang.correctness.permissions.file_permission.incorrect-default-permission -- directory
		t.Fatalf("mkdir: %v", err)
	}
	sidecar := `{"juggernaut": {"auth": {"mode": "iam", "region": "us-west-2", "profile": "dev-sidecar"}, "meta": {"managedBy": "juggernaut"}}}`
	if err := os.WriteFile(filepath.Join(home, ".config", "opencode", ".juggernaut.json"), []byte(sidecar), 0o600); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}
	t.Setenv("AWS_PROFILE", "")
	stubValidateProfileOK(t)
	r := doctor.NewReport()
	checkAWSProfile(r, provider.MustGet("opencode"), home, []string{"user"})

	if !strings.Contains(r.String(), "dev-sidecar") {
		t.Errorf("sidecar profile should be reported, got:\n%s", r.String())
	}
	if r.HasFailures() {
		t.Errorf("sidecar profile should not fail, report:\n%s", r.String())
	}
}
