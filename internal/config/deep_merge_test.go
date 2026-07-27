package config

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/testutil"
)

// TestMergeConfigPlan_DeepMergeKeys_PreservesSiblings is the data-loss
// regression: writing a nested-table key (e.g. Grok's "model") must merge only
// Juggernaut's own sub-key, preserving the user's other entries — NOT replace
// the whole table. Recreates the real scenario that would have wiped a user's
// batwing-coder / batmobile model profiles.
func TestMergeConfigPlan_DeepMergeKeys_PreservesSiblings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	m := NewManagerWithFormat(path, tomlFormat{})

	// User's pre-existing config with their own model profiles + default.
	if err := m.Write(map[string]any{
		"model": map[string]any{
			"batwing-coder":    map[string]any{"base_url": "http://192.168.0.131:8082/v1", "model": "qwen36"},
			"batmobile-gemma4": map[string]any{"base_url": "http://192.168.0.112:1234/v1"},
		},
		"models": map[string]any{"default": "batwing-coder"},
	}); err != nil {
		t.Fatal(err)
	}

	// Juggernaut writes its bedrock-grok block. Deep-merge keys: model, models.
	err := m.MergeConfigPlanDeep(map[string]any{
		"model": map[string]any{
			"bedrock-grok": map[string]any{"model": "xai.grok-4.3", "base_url": "https://mantle/openai/v1"},
		},
		"models": map[string]any{"default": "bedrock-grok"},
	}, []string{"model", "models"})
	if err != nil {
		t.Fatalf("MergeConfigPlanDeep: %v", err)
	}

	got, _ := m.Read()
	modelTbl, _ := got["model"].(map[string]any)
	if modelTbl == nil {
		t.Fatal("model table lost")
	}
	// Both the user's profiles AND ours must be present.
	for _, want := range []string{"batwing-coder", "batmobile-gemma4", "bedrock-grok"} {
		if _, ok := modelTbl[want]; !ok {
			t.Errorf("model.%s missing after merge — data loss!", want)
		}
	}
	// models.default: Juggernaut set it to bedrock-grok (a leaf override).
	modelsTbl, _ := got["models"].(map[string]any)
	if modelsTbl["default"] != "bedrock-grok" {
		t.Errorf("models.default = %v, want bedrock-grok", modelsTbl["default"])
	}
}

// TestRemoveManagedKeysDeep_PreservesSiblings is the uninstall-side data-loss
// regression: removing our bedrock-grok block must NOT delete the user's other
// model profiles or the whole table.
func TestRemoveManagedKeysDeep_PreservesSiblings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	m := NewManagerWithFormat(path, tomlFormat{})
	if err := m.Write(map[string]any{
		"model": map[string]any{
			"batwing-coder": map[string]any{"base_url": "http://x/v1"},
			"bedrock-grok":  map[string]any{"model": "xai.grok-4.3"},
		},
		"models": map[string]any{"default": "bedrock-grok", "web_search": "batwing-coder"},
	}); err != nil {
		t.Fatal(err)
	}
	err := m.RemoveManagedKeysDeep([]string{"model", "models"}, map[string][]string{
		"model":  {"bedrock-grok"},
		"models": {"default"},
	})
	if err != nil {
		t.Fatalf("RemoveManagedKeysDeep: %v", err)
	}
	got, _ := m.Read()
	modelTbl, _ := got["model"].(map[string]any)
	if modelTbl == nil {
		t.Fatal("model table wrongly deleted — data loss!")
	}
	if _, ok := modelTbl["batwing-coder"]; !ok {
		t.Error("user's batwing-coder profile lost on uninstall — data loss!")
	}
	if _, ok := modelTbl["bedrock-grok"]; ok {
		t.Error("our bedrock-grok should be removed")
	}
	modelsTbl, _ := got["models"].(map[string]any)
	if modelsTbl == nil {
		t.Fatal("models table wrongly deleted")
	}
	if _, ok := modelsTbl["default"]; ok {
		t.Error("our models.default should be removed")
	}
	if modelsTbl["web_search"] != "batwing-coder" {
		t.Error("user's models.web_search setting lost — data loss!")
	}
}

// TestRemoveManagedKeysDeep_EmptyTableCleaned: if removing our sub-key leaves the
// nested table empty, the table itself is removed (no orphan empty table).
func TestRemoveManagedKeysDeep_EmptyTableCleaned(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	m := NewManagerWithFormat(path, tomlFormat{})
	_ = m.Write(map[string]any{
		"model": map[string]any{"bedrock-grok": map[string]any{"model": "x"}},
	})
	_ = m.RemoveManagedKeysDeep([]string{"model"}, map[string][]string{"model": {"bedrock-grok"}})
	got, _ := m.Read()
	if _, ok := got["model"]; ok {
		t.Error("empty model table should be removed after our only sub-key is gone")
	}
}

// TestMergeConfigPlanDeep_NonMapValueFallsBackToReplace covers mergeNested's
// fallback: if a "deep" key's incoming value isn't a map, it's set whole.
func TestMergeConfigPlanDeep_NonMapValueFallsBackToReplace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	m := NewManagerWithFormat(path, tomlFormat{})
	_ = m.Write(map[string]any{"model": "old-string-value"})
	// "model" is declared deep but incoming value is a string → whole replace.
	if err := m.MergeConfigPlanDeep(map[string]any{"model": "new-string"}, []string{"model"}); err != nil {
		t.Fatal(err)
	}
	got, _ := m.Read()
	if got["model"] != "new-string" {
		t.Errorf("non-map deep value should replace, got %v", got["model"])
	}
}

// TestMergeConfigPlanDeep_ScalarUnderTableKeyErrors: if a deep-merge key is
// already present holding a NON-table value while we're merging a table into it,
// the config is corrupt/foreign for a key Juggernaut owns as a table. Rather
// than silently discard the user's value, merge must refuse with an actionable
// error and leave the file untouched.
func TestMergeConfigPlanDeep_ScalarUnderTableKeyErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	m := NewManagerWithFormat(path, tomlFormat{})
	_ = m.Write(map[string]any{"model": "legacy-scalar"})
	// Incoming is a TABLE (our bedrock-grok block) but existing "model" is a scalar.
	err := m.MergeConfigPlanDeep(map[string]any{
		"model": map[string]any{"bedrock-grok": map[string]any{"model": "xai.grok-4.3"}},
	}, []string{"model"})
	if err == nil {
		t.Fatal("expected an error merging a table onto an existing scalar, got nil")
	}
	if !strings.Contains(err.Error(), "expected a table") {
		t.Errorf("error should explain the type mismatch, got: %v", err)
	}
	// The user's scalar must survive untouched — we refused before writing.
	got, _ := m.Read()
	if got["model"] != "legacy-scalar" {
		t.Errorf("user's value must be preserved on refusal, got %v", got["model"])
	}
}

// TestRemoveManagedKeysDeep_ScalarUnderTableKeyErrors: uninstall must not
// silently no-op when a deep key holds a non-table value; it surfaces the
// corruption instead of leaving managed sub-keys unremoved.
func TestRemoveManagedKeysDeep_ScalarUnderTableKeyErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	m := NewManagerWithFormat(path, tomlFormat{})
	_ = m.Write(map[string]any{"model": "legacy-scalar"})
	err := m.RemoveManagedKeysDeep([]string{"model"}, map[string][]string{"model": {"bedrock-grok"}})
	if err == nil {
		t.Fatal("expected an error removing sub-keys from a scalar, got nil")
	}
	if !strings.Contains(err.Error(), "expected a table") {
		t.Errorf("error should explain the type mismatch, got: %v", err)
	}
}

// TestRemoveManagedKeysDeep_MissingTable is a no-op when the deep key is absent.
func TestRemoveManagedKeysDeep_MissingTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	m := NewManagerWithFormat(path, tomlFormat{})
	_ = m.Write(map[string]any{"unrelated": "x"})
	if err := m.RemoveManagedKeysDeep([]string{"model"}, map[string][]string{"model": {"bedrock-grok"}}); err != nil {
		t.Fatalf("should not error on missing table: %v", err)
	}
	got, _ := m.Read()
	if got["unrelated"] != "x" {
		t.Error("unrelated key must survive")
	}
}

// TestMergeConfigPlanDeep_NonDeepKeysStillReplace verifies keys NOT in the
// deep-merge set keep whole-value replace semantics (back-compat for Claude's
// env / modelOverrides etc.).
func TestMergeConfigPlanDeep_NonDeepKeysStillReplace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := NewManager(path)
	if err := m.Write(map[string]any{
		"env": map[string]any{"OLD_KEY": "stale"},
	}); err != nil {
		t.Fatal(err)
	}
	// env is NOT a deep-merge key → replaced wholesale.
	err := m.MergeConfigPlanDeep(map[string]any{
		"env": map[string]any{"AWS_REGION": "us-west-2"},
	}, nil) // no deep keys
	if err != nil {
		t.Fatalf("MergeConfigPlanDeep: %v", err)
	}
	got, _ := m.Read()
	env, _ := got["env"].(map[string]any)
	if _, stale := env["OLD_KEY"]; stale {
		t.Error("non-deep key should be replaced wholesale, stale OLD_KEY survived")
	}
	if env["AWS_REGION"] != "us-west-2" {
		t.Errorf("env not replaced correctly: %v", env)
	}
}

// TestMergeNested_RecursivePreservesUserSubKeys: when both existing and incoming
// have a map at a nested sub-key, the merge recurses so user values survive.
// This is the fix for the amazon-bedrock built-in provider: a user's
// profile under [model_providers.amazon-bedrock.aws] must not be overwritten
// by Juggernaut's region-only entry.
func TestMergeNested_RecursivePreservesUserSubKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	m := NewManagerWithFormat(path, tomlFormat{})
	if err := m.Write(map[string]any{
		"model_providers": map[string]any{
			"amazon-bedrock": map[string]any{
				"aws": map[string]any{
					"profile": "my-sso-profile",
					"region":  "us-west-2",
				},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	// Juggernaut applies — writes only region into the aws sub-table.
	err := m.MergeConfigPlanDeep(map[string]any{
		"model_providers": map[string]any{
			"amazon-bedrock": map[string]any{
				"aws": map[string]any{
					"region": "us-east-1",
				},
			},
		},
	}, []string{"model_providers"})
	if err != nil {
		t.Fatalf("MergeConfigPlanDeep: %v", err)
	}
	got, _ := m.Read()
	aws, ok := testutil.NestedMapChain(got, "model_providers", "amazon-bedrock", "aws")
	if !ok {
		t.Fatal("model_providers.amazon-bedrock.aws missing")
	}
	awsMap := aws.(map[string]any)
	// User's profile must survive the recursive merge.
	if awsMap["profile"] != "my-sso-profile" {
		t.Errorf("user profile lost after merge: %v", awsMap)
	}
	// Juggernaut's region should be written.
	if awsMap["region"] != "us-east-1" {
		t.Errorf("region not updated: %v", awsMap)
	}
}

// TestRemoveOwnedSubKeys_DotNotation: dot-notation paths (e.g.
// "amazon-bedrock.aws") remove only the nested sub-table, preserving sibling
// keys at the parent level.
func TestRemoveOwnedSubKeys_DotNotation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	m := NewManagerWithFormat(path, tomlFormat{})
	if err := m.Write(map[string]any{
		"model_providers": map[string]any{
			"amazon-bedrock": map[string]any{
				"aws": map[string]any{
					"profile": "my-sso-profile",
					"region":  "us-east-1",
				},
				"name": "Amazon Bedrock (Mantle)",
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	// Remove only the aws sub-table (not the entire amazon-bedrock entry).
	err := m.RemoveManagedKeysDeep([]string{"model_providers"}, map[string][]string{
		"model_providers": {"amazon-bedrock.aws"},
	})
	if err != nil {
		t.Fatalf("RemoveManagedKeysDeep: %v", err)
	}
	got, _ := m.Read()
	ab, ok := testutil.NestedMapChain(got, "model_providers", "amazon-bedrock")
	if !ok {
		t.Fatal("model_providers.amazon-bedrock missing")
	}
	abMap := ab.(map[string]any)
	// aws sub-table should be removed.
	if _, hasAws := abMap["aws"]; hasAws {
		t.Error("aws sub-table should be removed")
	}
	// Sibling keys under amazon-bedrock must survive.
	if abMap["name"] != "Amazon Bedrock (Mantle)" {
		t.Errorf("sibling name lost: %v", abMap)
	}
}

// TestRemoveOwnedSubKeys_DotNotationMissingIntermediate: when the intermediate
// path component doesn't exist, removal is a clean no-op (nothing to remove).
func TestRemoveOwnedSubKeys_DotNotationMissingIntermediate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	m := NewManagerWithFormat(path, tomlFormat{})
	if err := m.Write(map[string]any{
		"model_providers": map[string]any{
			"amazon-bedrock": map[string]any{
				"name": "Amazon Bedrock (Mantle)",
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	// "amazon-bedrock.aws" — aws doesn't exist → no-op, no error.
	err := m.RemoveManagedKeysDeep([]string{"model_providers"}, map[string][]string{
		"model_providers": {"amazon-bedrock.aws"},
	})
	if err != nil {
		t.Fatalf("should not error when intermediate key is missing: %v", err)
	}
	got, _ := m.Read()
	ab, ok := testutil.NestedMapChain(got, "model_providers", "amazon-bedrock")
	if !ok {
		t.Fatal("model_providers.amazon-bedrock missing")
	}
	if ab.(map[string]any)["name"] != "Amazon Bedrock (Mantle)" {
		t.Error("sibling name must survive")
	}
}

// TestRemoveOwnedSubKeys_DotNotationMissingDeepIntermediate: when a 3+ part path
// has a missing intermediate component (not the leaf), the recursive call hits
// the !present short-circuit (line 401-402 of manager.go) and returns nil.
func TestRemoveOwnedSubKeys_DotNotationMissingDeepIntermediate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	m := NewManagerWithFormat(path, tomlFormat{})
	if err := m.Write(map[string]any{
		"model_providers": map[string]any{
			"amazon-bedrock": map[string]any{
				"name":   "Amazon Bedrock (Mantle)",
				"region": "us-east-1",
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	// 3-part path: "amazon-bedrock.aws.credentials" — amazon-bedrock exists,
	// but "aws" does not → recursive call finds missing intermediate → no-op.
	err := m.RemoveManagedKeysDeep([]string{"model_providers"}, map[string][]string{
		"model_providers": {"amazon-bedrock.aws.credentials"},
	})
	if err != nil {
		t.Fatalf("should not error when deep intermediate is missing: %v", err)
	}
	got, _ := m.Read()
	ab, ok := testutil.NestedMapChain(got, "model_providers", "amazon-bedrock")
	if !ok {
		t.Fatal("model_providers.amazon-bedrock missing")
	}
	abMap := ab.(map[string]any)
	if abMap["name"] != "Amazon Bedrock (Mantle)" {
		t.Error("sibling name must survive")
	}
	if abMap["region"] != "us-east-1" {
		t.Error("sibling region must survive")
	}
}

// TestRemoveOwnedSubKeys_DotNotationNonMapIntermediate: when the intermediate
// path component is a scalar (not a map), removal errors with an actionable
// message instead of silently no-opping. Uses a 4-part path so the recursive
// call itself hits the type check and propagates the error back.
func TestRemoveOwnedSubKeys_DotNotationNonMapIntermediate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	m := NewManagerWithFormat(path, tomlFormat{})
	if err := m.Write(map[string]any{
		"model_providers": map[string]any{
			"amazon-bedrock": map[string]any{
				"aws": map[string]any{
					"credentials": "not-a-table", // 3rd level is a scalar
				},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	// "amazon-bedrock.aws.credentials.access_key" — aws is a map, credentials
	// is a scalar → recursive call errors at "credentials" and propagates back.
	err := m.RemoveManagedKeysDeep([]string{"model_providers"}, map[string][]string{
		"model_providers": {"amazon-bedrock.aws.credentials.access_key"},
	})
	if err == nil {
		t.Fatal("expected an error when intermediate is a scalar, got nil")
	}
	if !strings.Contains(err.Error(), "expected a table") {
		t.Errorf("error should explain the type mismatch, got: %v", err)
	}
}

// TestMergeNestedPrefix_RecursionGuard: the recursion guard (lines 268-270 of
// manager.go) only recurses when BOTH existing and incoming sub-keys are maps.
// If either side is not a map, the incoming value wins without recursion.
func TestMergeNestedPrefix_RecursionGuard(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	m := NewManagerWithFormat(path, tomlFormat{})

	// Existing: "aws" is a map with two keys.
	if err := m.Write(map[string]any{
		"model_providers": map[string]any{
			"amazon-bedrock": map[string]any{
				"aws": map[string]any{
					"profile": "my-profile",
					"region":  "us-west-2",
				},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	// Incoming: "aws" is a scalar — guard skips recursion, scalar replaces map.
	err := m.MergeConfigPlanDeep(map[string]any{
		"model_providers": map[string]any{
			"amazon-bedrock": map[string]any{
				"aws": "flat-string",
			},
		},
	}, []string{"model_providers"})
	if err != nil {
		t.Fatalf("MergeConfigPlanDeep: %v", err)
	}
	got, _ := m.Read()
	ab, ok := testutil.NestedMapChain(got, "model_providers", "amazon-bedrock")
	if !ok {
		t.Fatal("model_providers.amazon-bedrock missing")
	}
	if ab.(map[string]any)["aws"] != "flat-string" {
		t.Errorf("scalar should replace map (guard skipped recursion), got %v", ab.(map[string]any)["aws"])
	}

	// Reverse: existing is a scalar, incoming is a map — guard skips recursion,
	// incoming map replaces the scalar.
	_ = m.Write(map[string]any{
		"model_providers": map[string]any{
			"amazon-bedrock": map[string]any{
				"aws": "flat-string",
			},
		},
	})
	err = m.MergeConfigPlanDeep(map[string]any{
		"model_providers": map[string]any{
			"amazon-bedrock": map[string]any{
				"aws": map[string]any{"region": "us-east-1"},
			},
		},
	}, []string{"model_providers"})
	if err != nil {
		t.Fatalf("MergeConfigPlanDeep: %v", err)
	}
	got, _ = m.Read()
	aws, ok := testutil.NestedMapChain(got, "model_providers", "amazon-bedrock", "aws")
	if !ok {
		t.Fatal("model_providers.amazon-bedrock.aws missing")
	}
	if aws.(map[string]any)["region"] != "us-east-1" {
		t.Errorf("map replacement incorrect: %v", aws)
	}
}

// TestMergeNestedPrefix_IncomingNotMap: when the incoming value for a deep-merge
// key is not a map, fall back to whole-value set (the !ok branch).
func TestMergeNestedPrefix_IncomingNotMap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	m := NewManagerWithFormat(path, tomlFormat{})
	if err := m.Write(map[string]any{
		"model_providers": map[string]any{
			"amazon-bedrock": map[string]any{"region": "us-east-1"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	// model_providers is deep, but incoming value is a string → whole replace.
	err := m.MergeConfigPlanDeep(map[string]any{
		"model_providers": "just-a-string",
	}, []string{"model_providers"})
	if err != nil {
		t.Fatalf("MergeConfigPlanDeep: %v", err)
	}
	got, _ := m.Read()
	if got["model_providers"] != "just-a-string" {
		t.Errorf("non-map deep value should replace, got %v", got["model_providers"])
	}
}

// TestMergeNestedPrefix_RecursiveScalarOverrideMap: when existing has a map at a
// sub-key but incoming has a scalar, the scalar wins (no recursion).
func TestMergeNestedPrefix_RecursiveScalarOverrideMap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	m := NewManagerWithFormat(path, tomlFormat{})
	if err := m.Write(map[string]any{
		"model_providers": map[string]any{
			"amazon-bedrock": map[string]any{
				"aws": map[string]any{
					"profile": "my-profile",
					"region":  "us-west-2",
				},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	// Incoming aws is a scalar → overrides the entire aws map.
	err := m.MergeConfigPlanDeep(map[string]any{
		"model_providers": map[string]any{
			"amazon-bedrock": map[string]any{
				"aws": "override-scalar",
			},
		},
	}, []string{"model_providers"})
	if err != nil {
		t.Fatalf("MergeConfigPlanDeep: %v", err)
	}
	got, _ := m.Read()
	aws, ok := testutil.NestedMapChain(got, "model_providers", "amazon-bedrock", "aws")
	if !ok {
		t.Fatal("model_providers.amazon-bedrock.aws missing")
	}
	if aws != "override-scalar" {
		t.Errorf("scalar should override map, got %v", aws)
	}
}

// TestMergeNestedPrefix_DeeperRecursivePreservesUserSubKeys: verifies recursion
// works at multiple nesting levels, preserving user sub-keys at each depth.
func TestMergeNestedPrefix_DeeperRecursivePreservesUserSubKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	m := NewManagerWithFormat(path, tomlFormat{})
	if err := m.Write(map[string]any{
		"model_providers": map[string]any{
			"amazon-bedrock": map[string]any{
				"aws": map[string]any{
					"credentials": map[string]any{
						"profile": "my-sso-profile",
						"region":  "us-west-2",
					},
				},
				"name": "Amazon Bedrock (Mantle)",
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	// 4-level merge: model_providers → amazon-bedrock → aws → credentials.
	// User's profile survives; Juggernaut's region overrides.
	err := m.MergeConfigPlanDeep(map[string]any{
		"model_providers": map[string]any{
			"amazon-bedrock": map[string]any{
				"aws": map[string]any{
					"credentials": map[string]any{
						"region": "us-east-1",
					},
				},
			},
		},
	}, []string{"model_providers"})
	if err != nil {
		t.Fatalf("MergeConfigPlanDeep: %v", err)
	}
	got, _ := m.Read()
	creds, ok := testutil.NestedMapChain(got, "model_providers", "amazon-bedrock", "aws", "credentials")
	if !ok {
		t.Fatal("model_providers.amazon-bedrock.aws.credentials missing")
	}
	credsMap := creds.(map[string]any)
	if credsMap["profile"] != "my-sso-profile" {
		t.Errorf("user profile lost at deep level: %v", credsMap)
	}
	if credsMap["region"] != "us-east-1" {
		t.Errorf("region not overridden at deep level: %v", credsMap)
	}
	ab, ok := testutil.NestedMapChain(got, "model_providers", "amazon-bedrock")
	if !ok || ab.(map[string]any)["name"] != "Amazon Bedrock (Mantle)" {
		t.Errorf("sibling name lost")
	}
}

// TestRemoveOwnedSubKeys_DotNotationPreservesSiblings: removing a deeply nested
// key preserves sibling entries at all levels and exercises the successful
// recursive return path (line 407-416).
func TestRemoveOwnedSubKeys_DotNotationPreservesSiblings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	m := NewManagerWithFormat(path, tomlFormat{})
	if err := m.Write(map[string]any{
		"model_providers": map[string]any{
			"amazon-bedrock": map[string]any{
				"aws": map[string]any{
					"region":  "us-east-1",
					"profile": "my-profile",
				},
				"name": "Amazon Bedrock (Mantle)",
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	// Remove only the region — profile and name must survive.
	err := m.RemoveManagedKeysDeep([]string{"model_providers"}, map[string][]string{
		"model_providers": {"amazon-bedrock.aws.region"},
	})
	if err != nil {
		t.Fatalf("RemoveManagedKeysDeep: %v", err)
	}
	got, _ := m.Read()
	aws, ok := testutil.NestedMapChain(got, "model_providers", "amazon-bedrock", "aws")
	if !ok {
		t.Fatal("model_providers.amazon-bedrock.aws missing")
	}
	awsMap := aws.(map[string]any)
	if _, hasRegion := awsMap["region"]; hasRegion {
		t.Error("region should be removed")
	}
	if awsMap["profile"] != "my-profile" {
		t.Error("sibling profile must survive")
	}
	ab, ok := testutil.NestedMapChain(got, "model_providers", "amazon-bedrock")
	if !ok || ab.(map[string]any)["name"] != "Amazon Bedrock (Mantle)" {
		t.Error("sibling name must survive")
	}
}

// TestRemoveOwnedSubKeys_DotNotationEmptyParentCleaned: removing a nested key
// that empties its parent should clean up the parent map too.
func TestRemoveOwnedSubKeys_DotNotationEmptyParentCleaned(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	m := NewManagerWithFormat(path, tomlFormat{})
	if err := m.Write(map[string]any{
		"model_providers": map[string]any{
			"amazon-bedrock": map[string]any{
				"aws": map[string]any{"region": "us-east-1"},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	err := m.RemoveManagedKeysDeep([]string{"model_providers"}, map[string][]string{
		"model_providers": {"amazon-bedrock.aws"},
	})
	if err != nil {
		t.Fatalf("RemoveManagedKeysDeep: %v", err)
	}
	got, _ := m.Read()
	// amazon-bedrock becomes empty after aws is removed → cleaned up.
	mp, ok := got["model_providers"].(map[string]any)
	if ok && len(mp) > 0 {
		t.Errorf("empty model_providers should be cleaned up: %v", got)
	}
}
