package cmd

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/provider"
)

func TestApplyCodexMigratesMetadataToSidecar(t *testing.T) {
	for _, scope := range []string{"user", "project"} {
		t.Run(scope, func(t *testing.T) {
			home := setupApplyTestWithReset(t)
			mantleProjectDir(t)
			original := validateAWSProfile
			validateAWSProfile = func(context.Context, string, string) error { return nil }
			t.Cleanup(func() { validateAWSProfile = original })
			p := provider.MustGet("codex")
			mgr, err := newProviderManager(p, home, scope)
			if err != nil {
				t.Fatal(err)
			}
			legacy := map[string]any{
				"model_provider": "amazon-bedrock", "model": "openai.gpt-5.4",
				"mcp_servers": map[string]any{"aws-mcp": map[string]any{"command": "custom-proxy"}},
				"juggernaut": map[string]any{
					"meta": map[string]any{"managedBy": "juggernaut"},
					"auth": map[string]any{"mode": "iam", "profile": "developer", "region": "us-west-2", "endpoint": "mantle"},
				},
			}
			if err := mgr.Write(legacy); err != nil {
				t.Fatal(err)
			}
			for pass := range 2 {
				resetFlags()
				out := captureStdout(t, func() {
					if err := ExecuteArgs([]string{"apply", "--cli=codex", "--scope=" + scope, "--region=eu-west-1", "--model=openai.gpt-5.4"}); err != nil {
						t.Fatal(err)
					}
				})
				if strings.Contains(out, "Juggernaut v5") {
					t.Fatal("current mantle config misclassified as v5: " + out)
				}
				data, err := mgr.Read()
				if err != nil {
					t.Fatal(err)
				}
				if _, exists := data["juggernaut"]; exists {
					t.Fatalf("pass %d: metadata remains in Codex config", pass)
				}
				block, ok := provider.ReadScopeAuthBlock(p, home, scope, data)
				if !ok {
					t.Fatal("auth metadata was lost")
				}
				auth := block["auth"].(map[string]any)
				if auth["mode"] != "iam" || auth["profile"] != "developer" || auth["endpoint"] != "mantle" || auth["region"] != "us-east-1" {
					t.Fatalf("wrong auth metadata: %v", auth)
				}
				if data["mcp_servers"].(map[string]any)["aws-mcp"].(map[string]any)["command"] != "custom-proxy" {
					t.Fatal("user MCP config changed")
				}
			}
			resetFlags()
			if err := ExecuteArgs([]string{"uninstall", "--cli=codex", "--scope=" + scope, "--force"}); err != nil {
				t.Fatal(err)
			}
			if provider.SidecarExists(p, home, scope) {
				t.Fatal("uninstall left auth metadata behind")
			}
		})
	}
}

func TestCodexSidecarUsesResolvedRuntimeRegion(t *testing.T) {
	home := setupApplyTestWithReset(t)
	p := provider.MustGet("codex")
	if err := ExecuteArgs([]string{"apply", "--cli=codex", "--auth=iam", "--region=me-central-1", "--model=sol"}); err != nil {
		t.Fatal(err)
	}
	data, err := readProviderConfig(p, home, "user")
	if err != nil {
		t.Fatal(err)
	}
	block, ok := provider.ReadScopeAuthBlock(p, home, "user", data)
	if !ok {
		t.Fatal("missing sidecar")
	}
	region := block["auth"].(map[string]any)["region"]
	native := data["model_providers"].(map[string]any)["amazon-bedrock-runtime"].(map[string]any)["aws"].(map[string]any)["region"]
	if region != "us-east-1" || native != region {
		t.Fatalf("sidecar=%v native=%v; expected resolved us-east-1", region, native)
	}
}

func TestCodexSidecarWriteFailurePreservesLegacyConfig(t *testing.T) {
	home := setupApplyTestWithReset(t)
	p := provider.MustGet("codex")
	mgr, err := newProviderManager(p, home, "user")
	if err != nil {
		t.Fatal(err)
	}
	legacy := map[string]any{
		"model_provider": "amazon-bedrock-runtime", "model": "global.openai.gpt-6.1-sol",
		"juggernaut": map[string]any{"meta": map[string]any{"managedBy": "juggernaut"}, "auth": map[string]any{"mode": "iam", "region": "us-west-2"}},
	}
	if err := mgr.Write(legacy); err != nil {
		t.Fatal(err)
	}
	path, err := p.ConfigPath(home, "user")
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sidecar, err := p.(provider.SidecarAuthSource).SidecarPath(home, "user")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sidecar, 0o700); err != nil { // nosemgrep: go.lang.correctness.permissions.file_permission.incorrect-default-permission -- directory
		t.Fatal(err)
	}
	err = ExecuteArgs([]string{"apply", "--cli=codex", "--auth=iam"})
	if err == nil || !strings.Contains(err.Error(), "auth metadata sidecar") {
		t.Fatalf("expected sidecar failure, got %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("failed migration changed or removed the legacy auth state")
	}
}
