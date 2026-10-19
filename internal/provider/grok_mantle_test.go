package provider

import (
	"slices"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
	"github.com/jpvelasco/juggernaut/v5/internal/testutil"
)

// mantleGrokOpts returns apply-time options for a Grok mantle apply. The
// default region (us-west-2) serves both documented mantle models.
func mantleGrokOpts() Options {
	opts := baseOpts()
	opts.Endpoint = bedrock.EndpointMantle
	opts.AuthMode = authmode.BedrockAPIKey
	opts.Region = "us-west-2"
	opts.RegionExplicit = true
	return opts
}

func grokPlanModelBlock(t *testing.T, plan ConfigPlan) map[string]any {
	t.Helper()
	bg, ok := testutil.NestedMapChain(plan.Keys, "model", grokModelName)
	if !ok {
		t.Fatalf("[model.%s] missing", grokModelName)
	}
	block, ok := bg.(map[string]any)
	if !ok {
		t.Fatalf("[model.%s] not a map: %T", grokModelName, bg)
	}
	return block
}

// TestGrok_MantleBuildConfig: the default mantle config is the bare Grok 4.6
// foundation ID on the mantle base URL, using the Responses wire API, with the
// API-key auth-provider block and the endpoint persisted for launch/re-apply.
func TestGrok_MantleBuildConfig(t *testing.T) {
	p, _ := Get("grok")
	plan, err := p.BuildConfig(testConfig(), mantleGrokOpts())
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}

	block := grokPlanModelBlock(t, plan)
	want := map[string]any{
		"model":             "xai.grok-4.6",
		"base_url":          "https://bedrock-mantle.us-west-2.api.aws/openai/v1",
		"context_window":    500000,
		"api_backend":       "responses",
		"reasoning_summary": "none",
	}
	for k, v := range want {
		if block[k] != v {
			t.Errorf("[model.%s].%s = %v, want %v", grokModelName, k, block[k], v)
		}
	}
	if name, _ := block["name"].(string); !strings.Contains(name, "Mantle") {
		t.Errorf("name = %q, want it to mention Mantle", name)
	}
	if _, hasEnvKey := block["env_key"]; hasEnvKey {
		t.Error("env_key must NOT be set; the [auth] block replaces login")
	}

	auth, ok := testutil.NestedMapChain(plan.Keys, "auth")
	if !ok {
		t.Fatal("[auth] block missing")
	}
	authMap := auth.(map[string]any)
	if cmd, _ := authMap["auth_provider_command"].(string); !strings.Contains(cmd, "auth-token") {
		t.Errorf("auth_provider_command = %v, want `juggernaut auth-token`", authMap["auth_provider_command"])
	}

	for path, want := range map[string]string{
		"endpoint": bedrock.EndpointMantle,
		"mode":     authmode.BedrockAPIKey,
		"region":   "us-west-2",
	} {
		got, ok := testutil.NestedMapChain(plan.Keys, "juggernaut", "auth", path)
		if !ok || got != want {
			t.Errorf("juggernaut.auth.%s = %v, want %q", path, got, want)
		}
	}

	if len(plan.Warnings) != 0 {
		t.Errorf("served region should not warn, got %v", plan.Warnings)
	}
	if err := plan.Validate(); err != nil {
		t.Errorf("plan should validate: %v", err)
	}
}

// TestGrok_MantleModelForms: mantle accepts bare foundation IDs (and the
// grok-* shorthand / version suffix) and pins each model's card context
// window. The runtime default (4.7) is not reused and CRIS profiles are not
// written into a mantle config.
func TestGrok_MantleModelForms(t *testing.T) {
	p, _ := Get("grok")
	cases := []struct {
		in            string
		want          string
		contextWindow int
	}{
		{in: "", want: "xai.grok-4.6", contextWindow: 500000},
		{in: "xai.grok-4.6", want: "xai.grok-4.6", contextWindow: 500000},
		{in: "grok-4.6", want: "xai.grok-4.6", contextWindow: 500000},
		{in: "xai.grok-4.6:0", want: "xai.grok-4.6", contextWindow: 500000},
		{in: "xai.grok-4.3", want: "xai.grok-4.3", contextWindow: 1000000},
		{in: "grok-4.3", want: "xai.grok-4.3", contextWindow: 1000000},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			opts := mantleGrokOpts()
			opts.Model = tc.in
			plan, err := p.BuildConfig(testConfig(), opts)
			if err != nil {
				t.Fatalf("BuildConfig(%q): %v", tc.in, err)
			}
			block := grokPlanModelBlock(t, plan)
			if block["model"] != tc.want {
				t.Errorf("model = %v, want %v", block["model"], tc.want)
			}
			if block["context_window"] != tc.contextWindow {
				t.Errorf("context_window = %v, want %d", block["context_window"], tc.contextWindow)
			}
		})
	}
}

// TestGrok_MantleRejectsUnsupportedModels: a mantle config pins one region, so
// only models with verified mantle availability may be selected. CRIS profiles,
// runtime-only models, and unknown IDs are rejected before anything is written.
func TestGrok_MantleRejectsUnsupportedModels(t *testing.T) {
	p, _ := Get("grok")
	cases := []struct {
		in   string
		want string
	}{
		{in: "global.xai.grok-4.6", want: "cross-region inference profile"},
		{in: "us.xai.grok-4.3", want: `bare ID "xai.grok-4.3"`},
		{in: "global.xai.grok-9.9", want: "cross-region inference profile"},
		{in: "xai.grok-4.7", want: "not served on the mantle endpoint"},
		{in: "xai.grok-9.9", want: "unknown Grok mantle model"},
		{in: "anthropic.claude-sonnet-5", want: "unknown Grok mantle model"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			opts := mantleGrokOpts()
			opts.Model = tc.in
			_, err := p.BuildConfig(testConfig(), opts)
			if err == nil {
				t.Fatalf("BuildConfig(%q) accepted an unsupported mantle model", tc.in)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

// TestGrok_MantleIAMAuthMintsShortTermToken: mantle accepts the same
// short-term Bedrock API key as runtime, so an IAM apply must wire
// `auth-token --iam` rather than reject the endpoint.
func TestGrok_MantleIAMAuthMintsShortTermToken(t *testing.T) {
	p, _ := Get("grok")
	opts := mantleGrokOpts()
	opts.AuthMode = authmode.IAM
	plan, err := p.BuildConfig(testConfig(), opts)
	if err != nil {
		t.Fatalf("BuildConfig accepted IAM for mantle? got error: %v", err)
	}
	auth, ok := testutil.NestedMapChain(plan.Keys, "auth")
	if !ok {
		t.Fatal("[auth] block missing")
	}
	authMap := auth.(map[string]any)
	wantCmd := "juggernaut auth-token --iam --region=us-west-2"
	if cmd, _ := authMap["auth_provider_command"].(string); cmd != wantCmd {
		t.Errorf("auth_provider_command = %q, want %q", cmd, wantCmd)
	}
	if got, _ := testutil.NestedMapChain(plan.Keys, "juggernaut", "auth", "mode"); got != authmode.IAM {
		t.Errorf("juggernaut.auth.mode = %v, want iam", got)
	}
}

// TestGrok_MantleRegionPolicy: each model keeps a region the model card lists
// for mantle and is otherwise routed to that model's first verified region.
// The persisted juggernaut block records the final region.
func TestGrok_MantleRegionPolicy(t *testing.T) {
	p, _ := Get("grok")
	cases := []struct {
		name       string
		model      string
		region     string
		wantRegion string
		wantWarn   bool
	}{
		{"4.6 default region served", "", "us-west-2", "us-west-2", false},
		{"4.6 GovCloud region served", "xai.grok-4.6", "us-gov-east-1", "us-gov-east-1", false},
		{"4.6 runtime region overridden", "", "us-east-1", "us-west-2", true},
		{"4.3 east region served", "xai.grok-4.3", "us-east-1", "us-east-1", false},
		{"4.3 GovCloud West served", "xai.grok-4.3", "us-gov-west-1", "us-gov-west-1", false},
		{"4.3 unserved region overridden", "xai.grok-4.3", "eu-west-1", "us-west-2", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := mantleGrokOpts()
			opts.Model = tc.model
			opts.Region = tc.region
			plan, err := p.BuildConfig(testConfig(), opts)
			if err != nil {
				t.Fatalf("BuildConfig: %v", err)
			}
			block := grokPlanModelBlock(t, plan)
			wantBase := "https://bedrock-mantle." + tc.wantRegion + ".api.aws/openai/v1"
			if block["base_url"] != wantBase {
				t.Errorf("base_url = %v, want %q", block["base_url"], wantBase)
			}
			got, ok := testutil.NestedMapChain(plan.Keys, "juggernaut", "auth", "region")
			if !ok || got != tc.wantRegion {
				t.Errorf("juggernaut.auth.region = %v, want %q", got, tc.wantRegion)
			}
			if !tc.wantWarn {
				if len(plan.Warnings) != 0 {
					t.Errorf("expected no warning, got %v", plan.Warnings)
				}
				return
			}
			if len(plan.Warnings) != 1 {
				t.Fatalf("expected one warning, got %v", plan.Warnings)
			}
			if w := plan.Warnings[0]; !strings.Contains(w, "using "+tc.wantRegion+" instead") {
				t.Errorf("warning = %q, want it to name %s", w, tc.wantRegion)
			}
		})
	}
}

// TestGrok_RuntimeRejectsMantleOnlyModels: Grok 4.3 has no bedrock-runtime
// endpoint. A runtime apply (the default) must point at the mantle opt-in
// instead of writing a CRIS profile ID that fails at request time.
func TestGrok_RuntimeRejectsMantleOnlyModels(t *testing.T) {
	p, _ := Get("grok")
	for _, model := range []string{"xai.grok-4.3", "grok-4.3", "global.xai.grok-4.3"} {
		opts := baseOpts()
		opts.Model = model
		_, err := p.BuildConfig(testConfig(), opts)
		if err == nil {
			t.Fatalf("runtime BuildConfig accepted mantle-only model %q", model)
		}
		if !strings.Contains(err.Error(), "--endpoint=mantle") {
			t.Errorf("error = %v, want the --endpoint=mantle remedy", err)
		}
	}

	// Grok 4.6 is served on both endpoints and must stay selectable on runtime.
	opts := baseOpts()
	opts.Model = "xai.grok-4.6"
	if _, err := p.BuildConfig(testConfig(), opts); err != nil {
		t.Errorf("runtime BuildConfig rejected xai.grok-4.6: %v", err)
	}
}

// TestMantleRegionsFor: command-side region resolution uses the selected
// model's verified mantle regions when the provider has them, and falls back
// to the endpoint-wide list otherwise (Claude, invalid Grok selections).
func TestMantleRegionsFor(t *testing.T) {
	g, _ := Get("grok")
	claudeProv, _ := Get("claude")

	cases := []struct {
		name  string
		p     Provider
		model string
		want  []string
	}{
		{"grok default", g, "", []string{"us-west-2", "us-gov-east-1"}},
		{"grok 4.3 shorthand", g, "grok-4.3", []string{"us-west-2", "us-east-1", "us-east-2", "us-gov-west-1"}},
		{"grok runtime-only model", g, "xai.grok-4.7", bedrock.MantleRegions},
		{"grok invalid model", g, "bogus", bedrock.MantleRegions},
		{"claude has no per-model data", claudeProv, "anthropic.claude-sonnet-5", bedrock.MantleRegions},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := MantleRegionsFor(tc.p, tc.model); !slices.Equal(got, tc.want) {
				t.Errorf("MantleRegionsFor(%s, %q) = %v, want %v", tc.p.Name(), tc.model, got, tc.want)
			}
		})
	}
}

// TestGrok_AdvertisesMantleCapability pins the provider capability that gates
// --endpoint=mantle in cmd/.
func TestGrok_AdvertisesMantleCapability(t *testing.T) {
	g, _ := Get("grok")
	if !g.Supports(CapMantleEndpoint) {
		t.Error("grok must advertise CapMantleEndpoint")
	}
}

// TestGrok_SupportsModel_MantleSourceReason: a legacy mantle-source catalog
// entry is still unavailable to the runtime-default listing, but its reason
// must point at the mantle endpoint (not claim Grok no longer uses Mantle).
func TestGrok_SupportsModel_MantleSourceReason(t *testing.T) {
	p, _ := Get("grok")
	s := SupportsCatalogModel(p, CatalogModel{
		ID: "xai.grok-4.6", Source: "mantle", Status: "ACTIVE", Availability: "AVAILABLE",
	})
	if s.Supported {
		t.Error("mantle-source entry must not be supported on the runtime-default listing")
	}
	if !strings.Contains(s.Reason, "bedrock-mantle endpoint") {
		t.Errorf("reason = %q, want it to name the bedrock-mantle endpoint", s.Reason)
	}
}
