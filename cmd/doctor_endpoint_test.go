package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
	"github.com/jpvelasco/juggernaut/v5/internal/doctor"
)

func applyMantleForDoctor(t *testing.T) string {
	t.Helper()
	home := setupApplyTestWithReset(t)
	if err := ExecuteArgs([]string{
		"apply", "--cli=claude", "--auth=iam", "--region=us-west-2",
		"--endpoint=mantle", "--model=anthropic.claude-sonnet-5",
	}); err != nil {
		t.Fatalf("apply error: %v", err)
	}
	return home
}

func rewriteClaudeSettings(t *testing.T, home string, mutate func(map[string]any)) {
	t.Helper()
	settings := readClaudeSettings(t, home)
	mutate(settings)
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		t.Fatalf("marshaling settings: %v", err)
	}
	path := filepath.Join(home, ".claude", "settings.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("writing settings: %v", err)
	}
}

func swapMantleProbe(t *testing.T, res *bedrock.ConnectivityResult) *string {
	t.Helper()
	var calledWith string
	orig := mantleReachabilityProbe
	mantleReachabilityProbe = func(region string) *bedrock.ConnectivityResult {
		calledWith = region
		return res
	}
	t.Cleanup(func() { mantleReachabilityProbe = orig })
	return &calledWith
}

func TestCheckConnectivity_MantleProbesMantleEndpoint(t *testing.T) {
	home := applyMantleForDoctor(t)
	probe := swapMantleProbe(t, &bedrock.ConnectivityResult{
		OK:         true,
		AuthMode:   bedrock.EndpointMantle,
		Region:     "us-west-2",
		StatusCode: 401,
		Message:    "endpoint reachable (auth required)",
	})

	r := doctor.NewReport()
	checkConnectivity(r, home, "", []string{"user"})
	out := r.String()
	if !strings.Contains(out, "bedrock connectivity (mantle)") {
		t.Errorf("expected a mantle connectivity check, got: %s", out)
	}
	if !strings.Contains(out, "endpoint reachable") {
		t.Errorf("expected the probe detail, got: %s", out)
	}
	if *probe != "us-west-2" {
		t.Errorf("probe region = %q, want us-west-2", *probe)
	}
}

func TestCheckConnectivity_MantleUnsupportedRegion(t *testing.T) {
	home := applyMantleForDoctor(t)
	rewriteClaudeSettings(t, home, func(settings map[string]any) {
		settings["juggernaut"].(map[string]any)["auth"].(map[string]any)["region"] = "us-west-1"
	})

	called := false
	orig := mantleReachabilityProbe
	mantleReachabilityProbe = func(string) *bedrock.ConnectivityResult {
		called = true
		return &bedrock.ConnectivityResult{OK: true}
	}
	t.Cleanup(func() { mantleReachabilityProbe = orig })

	r := doctor.NewReport()
	checkConnectivity(r, home, "", []string{"user"})
	out := r.String()
	if !strings.Contains(out, "does not serve the mantle endpoint") {
		t.Errorf("expected an unsupported-region failure, got: %s", out)
	}
	if called {
		t.Error("probe should not run for a region without mantle")
	}
}

func TestCheckConnectivity_MantleMissingEnvWarns(t *testing.T) {
	home := applyMantleForDoctor(t)
	rewriteClaudeSettings(t, home, func(settings map[string]any) {
		delete(settings["env"].(map[string]any), "CLAUDE_CODE_USE_MANTLE")
	})
	swapMantleProbe(t, &bedrock.ConnectivityResult{OK: true, Message: "endpoint reachable"})

	r := doctor.NewReport()
	checkConnectivity(r, home, "", []string{"user"})
	out := r.String()
	if !strings.Contains(out, "CLAUDE_CODE_USE_MANTLE is not set") {
		t.Errorf("expected a stale-mantle-config warning, got: %s", out)
	}
}

func TestCheckConnectivity_UnknownEndpoint(t *testing.T) {
	home := applyMantleForDoctor(t)
	rewriteClaudeSettings(t, home, func(settings map[string]any) {
		settings["juggernaut"].(map[string]any)["auth"].(map[string]any)["endpoint"] = "bogus"
	})
	called := false
	orig := mantleReachabilityProbe
	mantleReachabilityProbe = func(string) *bedrock.ConnectivityResult {
		called = true
		return &bedrock.ConnectivityResult{OK: true}
	}
	t.Cleanup(func() { mantleReachabilityProbe = orig })

	r := doctor.NewReport()
	checkConnectivity(r, home, "", []string{"user"})
	out := r.String()
	if !strings.Contains(out, `unknown endpoint "bogus"`) {
		t.Errorf("expected an unknown-endpoint failure, got: %s", out)
	}
	if called {
		t.Error("probe should not run for an unknown endpoint")
	}
}

func TestCheckConnectivity_MantleAPIKeyMissingToken(t *testing.T) {
	home := applyMantleForDoctor(t)
	rewriteClaudeSettings(t, home, func(settings map[string]any) {
		settings["juggernaut"].(map[string]any)["auth"].(map[string]any)["mode"] = authmode.BedrockAPIKey
	})
	called := false
	orig := mantleReachabilityProbe
	mantleReachabilityProbe = func(string) *bedrock.ConnectivityResult {
		called = true
		return &bedrock.ConnectivityResult{OK: true}
	}
	t.Cleanup(func() { mantleReachabilityProbe = orig })

	r := doctor.NewReport()
	checkConnectivity(r, home, "", []string{"user"})
	out := r.String()
	if !strings.Contains(out, "bedrock API key not found in keychain") {
		t.Errorf("expected a missing-key failure, got: %s", out)
	}
	if called {
		t.Error("probe should not run without an API key")
	}
}
