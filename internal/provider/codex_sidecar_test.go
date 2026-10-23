package provider

import (
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
	"github.com/jpvelasco/juggernaut/v5/internal/testutil"
)

func TestCodexPlanExcludesJuggernautMetadata(t *testing.T) {
	p := MustGet("codex")
	for _, endpoint := range []string{bedrock.EndpointRuntime, bedrock.EndpointMantle} {
		t.Run(endpoint, func(t *testing.T) {
			plan, err := p.BuildConfig(nil, Options{Endpoint: endpoint, Region: "us-west-2", AuthMode: "iam", Profile: "developer"})
			if err != nil {
				t.Fatal(err)
			}
			if _, exists := plan.Keys["juggernaut"]; exists {
				t.Fatal("Codex rejects the in-file juggernaut metadata key")
			}
			if !HasSidecar(p) {
				t.Fatal("Codex must retain auth metadata in a sidecar")
			}
			if err := plan.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCodexSidecarScopeIsolation(t *testing.T) {
	home := testutil.NewTestHome(t)
	t.Chdir(t.TempDir())
	p := MustGet("codex")
	for _, scope := range []string{"user", "project"} {
		mode := "iam"
		if scope == "project" {
			mode = "bedrock-api-key"
		}
		if err := WriteSidecar(p, home, Options{Scope: scope, AuthMode: mode, Region: "us-west-2"}); err != nil {
			t.Fatal(err)
		}
	}
	user, userOK := ReadScopeAuthBlock(p, home, "user", nil)
	project, projectOK := ReadScopeAuthBlock(p, home, "project", nil)
	if !userOK || !projectOK || BlockAuthMode(user) != "iam" || BlockAuthMode(project) != "bedrock-api-key" {
		t.Fatal("Codex scopes shared or lost auth metadata")
	}
	if err := RemoveSidecar(p, home, []string{"user"}); err != nil {
		t.Fatal(err)
	}
	if SidecarExists(p, home, "user") || !SidecarExists(p, home, "project") {
		t.Fatal("user uninstall affected project auth metadata")
	}
}
