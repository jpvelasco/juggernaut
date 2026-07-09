package config

import "testing"

// TestDetectCollisions_EmptyExisting_NoCollisions: a brand-new file has nothing
// to collide with.
func TestDetectCollisions_EmptyExisting_NoCollisions(t *testing.T) {
	got := DetectCollisions(map[string]any{}, map[string]any{
		"model": "us.anthropic.claude-sonnet-5",
	}, nil, nil)
	if len(got) != 0 {
		t.Errorf("expected no collisions on empty existing config, got %v", got)
	}
}

// TestDetectCollisions_JuggernautKeyNeverChecked: the "juggernaut" key is
// always owned by Juggernaut outright, collision or not.
func TestDetectCollisions_JuggernautKeyNeverChecked(t *testing.T) {
	got := DetectCollisions(map[string]any{
		"juggernaut": map[string]any{"meta": map[string]any{"managedBy": "someone-else"}},
	}, map[string]any{
		"juggernaut": map[string]any{"meta": map[string]any{"managedBy": "juggernaut"}},
	}, nil, nil)
	if len(got) != 0 {
		t.Errorf("juggernaut key must never be checked for collisions, got %v", got)
	}
}

// TestDetectCollisions_ScalarKeyPresent_Collision: a whole-value scalar key
// (e.g. Claude's "model", Codex's "model_provider") that already has a
// foreign value is a collision.
func TestDetectCollisions_ScalarKeyPresent_Collision(t *testing.T) {
	got := DetectCollisions(map[string]any{
		"model": "my-own-model-id",
	}, map[string]any{
		"model": "us.anthropic.claude-sonnet-5",
	}, nil, nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 collision, got %v", got)
	}
	if got[0].Path != "model" {
		t.Errorf("expected path %q, got %q", "model", got[0].Path)
	}
	if got[0].Existing != "my-own-model-id" {
		t.Errorf("expected existing value %q, got %v", "my-own-model-id", got[0].Existing)
	}
}

// TestDetectCollisions_ScalarKeyAbsent_NoCollision: a scalar key with nothing
// pre-existing is not a collision.
func TestDetectCollisions_ScalarKeyAbsent_NoCollision(t *testing.T) {
	got := DetectCollisions(map[string]any{
		"unrelated": "x",
	}, map[string]any{
		"model": "us.anthropic.claude-sonnet-5",
	}, nil, nil)
	if len(got) != 0 {
		t.Errorf("expected no collisions, got %v", got)
	}
}

// TestDetectCollisions_MapWholeValueKey_SubKeyCollision: Claude's "env" is a
// whole-value key that is itself a map — collision detection must be
// fine-grained at the sub-key Juggernaut is about to write, not the whole map.
func TestDetectCollisions_MapWholeValueKey_SubKeyCollision(t *testing.T) {
	got := DetectCollisions(map[string]any{
		"env": map[string]any{"AWS_REGION": "eu-west-1"},
	}, map[string]any{
		"env": map[string]any{"AWS_REGION": "us-west-2"},
	}, nil, nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 collision, got %v", got)
	}
	if got[0].Path != "env.AWS_REGION" {
		t.Errorf("expected path %q, got %q", "env.AWS_REGION", got[0].Path)
	}
	if got[0].Existing != "eu-west-1" {
		t.Errorf("expected existing value %q, got %v", "eu-west-1", got[0].Existing)
	}
}

// TestDetectCollisions_StringMapWholeValueKey_SubKeyCollision: Claude's real
// plan shape passes "env" as map[string]string (schema.NativeKeys.Env), not
// map[string]any — collision detection must handle this concrete type too.
func TestDetectCollisions_StringMapWholeValueKey_SubKeyCollision(t *testing.T) {
	got := DetectCollisions(map[string]any{
		"env": map[string]any{"AWS_REGION": "eu-west-1"},
	}, map[string]any{
		"env": map[string]string{"AWS_REGION": "us-west-2"},
	}, nil, nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 collision, got %v", got)
	}
	if got[0].Path != "env.AWS_REGION" {
		t.Errorf("expected path %q, got %q", "env.AWS_REGION", got[0].Path)
	}
}

// TestDetectCollisions_MapWholeValueKey_SiblingSubKeyNoCollision: a user's
// unrelated env var sitting next to what Juggernaut writes must never trigger
// a collision — only the exact sub-key Juggernaut owns is checked.
func TestDetectCollisions_MapWholeValueKey_SiblingSubKeyNoCollision(t *testing.T) {
	got := DetectCollisions(map[string]any{
		"env": map[string]any{"NODE_ENV": "production"},
	}, map[string]any{
		"env": map[string]any{"AWS_REGION": "us-west-2"},
	}, nil, nil)
	if len(got) != 0 {
		t.Errorf("sibling env var must not collide, got %v", got)
	}
}

// TestDetectCollisions_MapWholeValueKeyTypeMismatch_Collision: if the whole
// key exists but is not a map while Juggernaut wants to write a map there,
// that mismatch itself is a collision (the merge would fail this way too).
func TestDetectCollisions_MapWholeValueKeyTypeMismatch_Collision(t *testing.T) {
	got := DetectCollisions(map[string]any{
		"env": "not-a-map",
	}, map[string]any{
		"env": map[string]any{"AWS_REGION": "us-west-2"},
	}, nil, nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 collision for type mismatch, got %v", got)
	}
	if got[0].Path != "env" {
		t.Errorf("expected path %q, got %q", "env", got[0].Path)
	}
}

// TestDetectCollisions_PermissionsDefaultModeCollision: "permissions" is
// special-cased to only the defaultMode leaf, mirroring mergePermissions.
func TestDetectCollisions_PermissionsDefaultModeCollision(t *testing.T) {
	got := DetectCollisions(map[string]any{
		"permissions": map[string]any{"defaultMode": "acceptEdits"},
	}, map[string]any{
		"permissions": map[string]any{"defaultMode": "auto"},
	}, nil, nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 collision, got %v", got)
	}
	if got[0].Path != "permissions.defaultMode" {
		t.Errorf("expected path %q, got %q", "permissions.defaultMode", got[0].Path)
	}
	if got[0].Existing != "acceptEdits" {
		t.Errorf("expected existing value %q, got %v", "acceptEdits", got[0].Existing)
	}
}

// TestDetectCollisions_PermissionsTypeMismatch_Collision: if "permissions"
// exists but is not a map (corrupted/foreign config), mergePermissions'
// `existing["permissions"].(map[string]any)` assertion would silently yield
// nil and the foreign value would be clobbered with no error at merge time —
// so the type mismatch itself must be reported as a collision, mirroring the
// map-whole-value-key type-mismatch handling.
func TestDetectCollisions_PermissionsTypeMismatch_Collision(t *testing.T) {
	got := DetectCollisions(map[string]any{
		"permissions": "not-a-map",
	}, map[string]any{
		"permissions": map[string]any{"defaultMode": "auto"},
	}, nil, nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 collision for type mismatch, got %v", got)
	}
	if got[0].Path != "permissions" {
		t.Errorf("expected path %q, got %q", "permissions", got[0].Path)
	}
	if got[0].Existing != "not-a-map" {
		t.Errorf("expected existing value %q, got %v", "not-a-map", got[0].Existing)
	}
}

// TestDetectCollisions_PermissionsOtherRulesNoCollision: a user's own
// allow/deny/ask permission rules must never collide, even when Juggernaut
// writes defaultMode into the same table.
func TestDetectCollisions_PermissionsOtherRulesNoCollision(t *testing.T) {
	got := DetectCollisions(map[string]any{
		"permissions": map[string]any{"allow": []any{"Bash(ls:*)"}},
	}, map[string]any{
		"permissions": map[string]any{"defaultMode": "auto"},
	}, nil, nil)
	if len(got) != 0 {
		t.Errorf("user's own permission rules must not collide, got %v", got)
	}
}

// TestDetectCollisions_DeepMergeKey_OwnedSubKeyCollision: Grok's
// [model.bedrock-grok] already existing (e.g. a prior unrelated user profile
// with that exact name) is a collision.
func TestDetectCollisions_DeepMergeKey_OwnedSubKeyCollision(t *testing.T) {
	got := DetectCollisions(map[string]any{
		"model": map[string]any{"bedrock-grok": map[string]any{"model": "their-own-thing"}},
	}, map[string]any{
		"model": map[string]any{"bedrock-grok": map[string]any{"model": "xai.grok-4.3"}},
	}, []string{"model"}, map[string][]string{"model": {"bedrock-grok"}})
	if len(got) != 1 {
		t.Fatalf("expected 1 collision, got %v", got)
	}
	if got[0].Path != "model.bedrock-grok" {
		t.Errorf("expected path %q, got %q", "model.bedrock-grok", got[0].Path)
	}
}

// TestDetectCollisions_DeepMergeKey_SiblingEntryNoCollision: a user's own
// sibling model profile in the same deep-merge table must never collide.
func TestDetectCollisions_DeepMergeKey_SiblingEntryNoCollision(t *testing.T) {
	got := DetectCollisions(map[string]any{
		"model": map[string]any{"my-own-profile": map[string]any{"base_url": "http://x/v1"}},
	}, map[string]any{
		"model": map[string]any{"bedrock-grok": map[string]any{"model": "xai.grok-4.3"}},
	}, []string{"model"}, map[string][]string{"model": {"bedrock-grok"}})
	if len(got) != 0 {
		t.Errorf("sibling model profile must not collide, got %v", got)
	}
}

// TestDetectCollisions_DeepMergeKey_DottedPathCollision: Codex's dotted owned
// sub-key path (model_providers.amazon-bedrock.aws.region) is checked at the
// exact leaf.
func TestDetectCollisions_DeepMergeKey_DottedPathCollision(t *testing.T) {
	got := DetectCollisions(map[string]any{
		"model_providers": map[string]any{
			"amazon-bedrock": map[string]any{
				"aws": map[string]any{"region": "eu-west-1"},
			},
		},
	}, map[string]any{
		"model_providers": map[string]any{
			"amazon-bedrock": map[string]any{
				"aws": map[string]any{"region": "us-east-1"},
			},
		},
	}, []string{"model_providers"}, map[string][]string{"model_providers": {"amazon-bedrock.aws.region"}})
	if len(got) != 1 {
		t.Fatalf("expected 1 collision, got %v", got)
	}
	if got[0].Path != "model_providers.amazon-bedrock.aws.region" {
		t.Errorf("expected path %q, got %q", "model_providers.amazon-bedrock.aws.region", got[0].Path)
	}
	if got[0].Existing != "eu-west-1" {
		t.Errorf("expected existing value %q, got %v", "eu-west-1", got[0].Existing)
	}
}

// TestDetectCollisions_DeepMergeKey_DottedPathMissingIntermediateNoCollision:
// when an intermediate path component is absent, there's nothing to collide
// with — consistent with removeNestedKey's missing-intermediate no-op.
func TestDetectCollisions_DeepMergeKey_DottedPathMissingIntermediateNoCollision(t *testing.T) {
	got := DetectCollisions(map[string]any{
		"model_providers": map[string]any{
			"amazon-bedrock": map[string]any{"name": "their config"},
		},
	}, map[string]any{
		"model_providers": map[string]any{
			"amazon-bedrock": map[string]any{
				"aws": map[string]any{"region": "us-east-1"},
			},
		},
	}, []string{"model_providers"}, map[string][]string{"model_providers": {"amazon-bedrock.aws.region"}})
	if len(got) != 0 {
		t.Errorf("missing intermediate path must not collide, got %v", got)
	}
}

// TestDetectCollisions_DeepMergeKey_RootTypeMismatch_Collision: if the
// deep-merge key's root exists but is not a map (e.g. Grok's "model": "x"
// instead of a table), that mismatch itself must be reported — mergeNested's
// type check would refuse the merge for the same reason, but collision
// detection should surface it up front with the intended refusal message
// rather than a bare merge-time type error.
func TestDetectCollisions_DeepMergeKey_RootTypeMismatch_Collision(t *testing.T) {
	got := DetectCollisions(map[string]any{
		"model": "not-a-table",
	}, map[string]any{
		"model": map[string]any{"bedrock-grok": map[string]any{"model": "xai.grok-4.3"}},
	}, []string{"model"}, map[string][]string{"model": {"bedrock-grok"}})
	if len(got) != 1 {
		t.Fatalf("expected 1 collision for type mismatch, got %v", got)
	}
	if got[0].Path != "model" {
		t.Errorf("expected path %q, got %q", "model", got[0].Path)
	}
	if got[0].Existing != "not-a-table" {
		t.Errorf("expected existing value %q, got %v", "not-a-table", got[0].Existing)
	}
}

// TestDetectCollisions_DeepMergeKey_DottedPathIntermediateTypeMismatch_Collision:
// if an INTERMEDIATE component of a dotted owned sub-key path exists but is
// not a map (e.g. Codex's model_providers.amazon-bedrock is a string instead
// of a table), mergeNestedPrefix's recursion guard would silently let
// Juggernaut's map replace the foreign scalar with no error — so the
// mismatch must be surfaced as a collision, not treated like a genuinely
// missing intermediate.
func TestDetectCollisions_DeepMergeKey_DottedPathIntermediateTypeMismatch_Collision(t *testing.T) {
	got := DetectCollisions(map[string]any{
		"model_providers": map[string]any{
			"amazon-bedrock": "not-a-table",
		},
	}, map[string]any{
		"model_providers": map[string]any{
			"amazon-bedrock": map[string]any{
				"aws": map[string]any{"region": "us-east-1"},
			},
		},
	}, []string{"model_providers"}, map[string][]string{"model_providers": {"amazon-bedrock.aws.region"}})
	if len(got) != 1 {
		t.Fatalf("expected 1 collision for intermediate type mismatch, got %v", got)
	}
	if got[0].Path != "model_providers.amazon-bedrock.aws.region" {
		t.Errorf("expected path %q, got %q", "model_providers.amazon-bedrock.aws.region", got[0].Path)
	}
	if got[0].Existing != "not-a-table" {
		t.Errorf("expected existing value %q, got %v", "not-a-table", got[0].Existing)
	}
}

// TestDetectCollisions_NoEqualityShortcut: even when the existing foreign
// value is byte-identical to what Juggernaut would write, presence alone
// triggers the collision — equality is not a silent "already fine" escape.
func TestDetectCollisions_NoEqualityShortcut(t *testing.T) {
	got := DetectCollisions(map[string]any{
		"model": "us.anthropic.claude-sonnet-5",
	}, map[string]any{
		"model": "us.anthropic.claude-sonnet-5",
	}, nil, nil)
	if len(got) != 1 {
		t.Errorf("identical value must still collide, got %v", got)
	}
}

// TestDetectCollisions_ScalarArrayKeyPresent_Collision: array-valued
// whole-value keys (e.g. fallbackModel) collide on bare presence, same as
// scalars — no sub-structure to be fine-grained about.
func TestDetectCollisions_ScalarArrayKeyPresent_Collision(t *testing.T) {
	got := DetectCollisions(map[string]any{
		"fallbackModel": []any{"their-fallback"},
	}, map[string]any{
		"fallbackModel": []string{"us.anthropic.claude-haiku-4-5"},
	}, nil, nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 collision, got %v", got)
	}
	if got[0].Path != "fallbackModel" {
		t.Errorf("expected path %q, got %q", "fallbackModel", got[0].Path)
	}
}

// TestDetectCollisions_MultipleCollisions_SortedByPath: output order must be
// deterministic regardless of map iteration order, so error messages and
// tests are stable.
func TestDetectCollisions_MultipleCollisions_SortedByPath(t *testing.T) {
	got := DetectCollisions(map[string]any{
		"model":         "their-model",
		"fallbackModel": []any{"their-fallback"},
		"env":           map[string]any{"AWS_REGION": "eu-west-1"},
	}, map[string]any{
		"model":         "us.anthropic.claude-sonnet-5",
		"fallbackModel": []string{"us.anthropic.claude-haiku-4-5"},
		"env":           map[string]any{"AWS_REGION": "us-west-2"},
	}, nil, nil)
	if len(got) != 3 {
		t.Fatalf("expected 3 collisions, got %v", got)
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].Path >= got[i].Path {
			t.Errorf("collisions not sorted by path: %v", got)
			break
		}
	}
}
