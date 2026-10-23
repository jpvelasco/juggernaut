package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
	"github.com/jpvelasco/juggernaut/v5/internal/doctor"
	"github.com/jpvelasco/juggernaut/v5/internal/provider"
)

func mantleProjectDir(t *testing.T) {
	t.Helper()
	data, err := os.ReadFile(findBedrockConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	writeTempBedrockConfig(t, dir, string(data))
	chdirTo(t, dir)
}

func TestApplyOpenAIMantleRoundTrip(t *testing.T) {
	for _, cli := range []string{"codex", "opencode"} {
		for _, scope := range []string{"user", "project"} {
			t.Run(cli+"/"+scope, func(t *testing.T) {
				home := setupApplyTestWithReset(t)
				mantleProjectDir(t)
				p := provider.MustGet(cli)
				mgr, err := newProviderManager(p, home, scope)
				if err != nil {
					t.Fatal(err)
				}
				if err := mgr.Write(map[string]any{"custom": "keep", "providers": map[string]any{"user-provider": map[string]any{"settings": map[string]any{"custom": "keep"}}}}); err != nil {
					t.Fatal(err)
				}
				apply := func(extra ...string) string {
					t.Helper()
					resetFlags()
					args := append([]string{"apply", "--cli=" + cli, "--scope=" + scope, "--auth=iam", "--region=us-west-2"}, extra...)
					out := captureStdout(t, func() {
						if err := ExecuteArgs(args); err != nil {
							t.Fatal(err)
						}
					})
					return out
				}
				apply("--endpoint=runtime")
				apply("--endpoint=mantle", "--model=openai.gpt-5.4")
				data, err := mgr.Read()
				if err != nil {
					t.Fatal(err)
				}
				if isLegacyProviderConfig(p, home, scope, data) {
					t.Fatal("current mantle config misclassified as v5")
				}
				checker := p.(provider.MantleConfigChecker)
				if err := checker.ValidateMantleConfig(data, "us-west-2"); err != nil {
					t.Fatal(err)
				}
				probe := swapMantleProbe(t, &bedrock.ConnectivityResult{OK: true, Message: "endpoint reachable"})
				r := doctor.NewReport()
				checkProviderMantle(r, p, home, "", []string{scope})
				if r.HasFailures() || *probe != "us-west-2" {
					t.Fatalf("doctor: %s, probe=%s", r.String(), *probe)
				}
				if out := apply(); strings.Contains(out, "Juggernaut v5") {
					t.Fatal(out)
				}
				data, err = mgr.Read()
				if err != nil {
					t.Fatal(err)
				}
				block, ok := provider.ReadScopeAuthBlock(p, home, scope, data)
				if !ok || provider.BlockEndpoint(block) != "mantle" {
					t.Fatal("plain re-apply lost mantle endpoint")
				}
				apply("--endpoint=runtime")
				data, err = mgr.Read()
				if err != nil {
					t.Fatal(err)
				}
				if data["custom"] != "keep" {
					t.Fatal("lost user setting")
				}
				providers := data["providers"].(map[string]any)
				if _, ok := providers["user-provider"]; !ok {
					t.Fatal("lost foreign provider")
				}
				if _, ok := providers["amazon-bedrock-mantle"]; ok {
					t.Fatal("inactive mantle provider survived runtime switch")
				}
				if err := mgr.RemoveManagedKeysDeep(p.NativeManagedKeys(), p.OwnedSubKeys()); err != nil {
					t.Fatal(err)
				}
				data, err = mgr.Read()
				if err != nil {
					t.Fatal(err)
				}
				if data["custom"] != "keep" {
					t.Fatal("uninstall removed user settings")
				}
			})
		}
	}
}

func TestApplyCodexLegacyToMantle(t *testing.T) {
	home := setupApplyTestWithReset(t)
	p := provider.MustGet("codex")
	mgr, err := newProviderManager(p, home, "user")
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.Write(map[string]any{
		"model_provider": "amazon-bedrock", "model": "openai.gpt-5.4",
		"model_providers": map[string]any{"amazon-bedrock": map[string]any{"base_url": "https://bedrock-mantle.us-west-2.api.aws/v1", "env_key": "OLD_KEY", "wire_api": "chat", "custom": "keep"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := ExecuteArgs([]string{"apply", "--cli=codex", "--auth=iam", "--endpoint=mantle", "--model=openai.gpt-5.4"}); err != nil {
		t.Fatal(err)
	}
	data, err := mgr.Read()
	if err != nil {
		t.Fatal(err)
	}
	if err := p.(provider.MantleConfigChecker).ValidateMantleConfig(data, "us-west-2"); err != nil {
		t.Fatal(err)
	}
	entry := data["model_providers"].(map[string]any)["amazon-bedrock"].(map[string]any)
	if entry["custom"] != "keep" {
		t.Fatal("lost unknown native provider setting")
	}
}

func TestOpenCodeEndpointUsesSelectedScope(t *testing.T) {
	home := setupApplyTestWithReset(t)
	mantleProjectDir(t)
	p := provider.MustGet("opencode")
	for _, scope := range []string{"user", "project"} {
		resetFlags()
		endpoint := "runtime"
		if scope == "project" {
			endpoint = "mantle"
		}
		if err := ExecuteArgs([]string{"apply", "--cli=opencode", "--scope=" + scope, "--auth=iam", "--endpoint=" + endpoint}); err != nil {
			t.Fatal(err)
		}
	}
	resetFlags()
	applyFlags.scope = "user"
	if endpoint, err := resolveApplyEndpoint(home, p); err != nil || endpoint != "runtime" {
		t.Fatalf("user endpoint = %s, %v", endpoint, err)
	}
	applyFlags.scope = "project"
	if endpoint, err := resolveApplyEndpoint(home, p); err != nil || endpoint != "mantle" {
		t.Fatalf("project endpoint = %s, %v", endpoint, err)
	}
}

func TestDoctorOpenAIMantleDriftAndMissingKey(t *testing.T) {
	for _, cli := range []string{"codex", "opencode"} {
		for _, drift := range []string{"model", "endpoint", "key"} {
			t.Run(cli+"/"+drift, func(t *testing.T) {
				home := setupApplyTestWithReset(t)
				p := provider.MustGet(cli)
				if err := ExecuteArgs([]string{"apply", "--cli=" + cli, "--auth=iam", "--endpoint=mantle"}); err != nil {
					t.Fatal(err)
				}
				mgr, err := newProviderManager(p, home, "user")
				if err != nil {
					t.Fatal(err)
				}
				data, err := mgr.Read()
				if err != nil {
					t.Fatal(err)
				}
				block, ok := provider.ReadScopeAuthBlock(p, home, "user", data)
				if !ok {
					t.Fatal("missing metadata")
				}
				if drift == "model" {
					data["model"] = "bogus"
				}
				endpoint, mode := "mantle", "iam"
				if drift == "endpoint" {
					endpoint = "bogus"
				}
				if drift == "key" {
					mode = "bedrock-api-key"
				}
				auth := block["auth"].(map[string]any)
				auth["endpoint"], auth["mode"] = endpoint, mode
				if provider.HasSidecar(p) {
					if err := provider.WriteSidecar(p, home, provider.Options{Scope: "user", Endpoint: endpoint, AuthMode: mode, Region: "us-west-2"}); err != nil {
						t.Fatal(err)
					}
				} else {
					data["juggernaut"] = block
				}
				if err := mgr.Write(data); err != nil {
					t.Fatal(err)
				}
				probe := swapMantleProbe(t, &bedrock.ConnectivityResult{OK: true})
				r := doctor.NewReport()
				checkProviderMantle(r, p, home, "", []string{"user"})
				if !r.HasFailures() || *probe != "" {
					t.Fatalf("invalid state probed or passed: %s", r.String())
				}
			})
		}
	}
}
