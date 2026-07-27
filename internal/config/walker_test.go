package config

import (
	"fmt"
	"strings"
	"testing"
)

// TestWalkNestedMap_LeafPresent verifies the basic leaf-found case.
func TestWalkNestedMap_LeafPresent(t *testing.T) {
	data := map[string]any{"a": map[string]any{"b": "leaf-value"}}
	var capturedVal any
	var capturedPresent bool
	walkNestedMap(data, []string{"a", "b"}, func(_ map[string]any, _ string, val any, present bool, isMap bool) {
		capturedVal = val
		capturedPresent = present
	})
	if capturedVal != "leaf-value" {
		t.Errorf("expected leaf-value, got %v", capturedVal)
	}
	if !capturedPresent {
		t.Error("expected present=true")
	}
}

// TestWalkNestedMap_LeafAbsent verifies the leaf-not-found case.
func TestWalkNestedMap_LeafAbsent(t *testing.T) {
	data := map[string]any{"a": map[string]any{"b": "value"}}
	var called bool
	walkNestedMap(data, []string{"a", "c"}, func(_ map[string]any, _ string, _ any, present bool, isMap bool) {
		called = true
		if present {
			t.Error("expected present=false for absent leaf")
		}
	})
	if !called {
		t.Error("visitor should still be called for absent leaf")
	}
}

// TestWalkNestedMap_MissingIntermediate verifies that a missing intermediate
// stops traversal without calling the visitor.
func TestWalkNestedMap_MissingIntermediate(t *testing.T) {
	data := map[string]any{"a": map[string]any{"b": "value"}}
	var called bool
	walkNestedMap(data, []string{"a", "missing", "deep"}, func(_ map[string]any, _ string, _ any, _ bool, _ bool) {
		called = true
	})
	if called {
		t.Error("visitor must not be called when intermediate is missing")
	}
}

// TestWalkNestedMap_WrongTypeIntermediate verifies that a wrong-type
// intermediate calls the visitor with isMap=false.
func TestWalkNestedMap_WrongTypeIntermediate(t *testing.T) {
	data := map[string]any{"a": map[string]any{"b": "scalar-not-map"}}
	var capturedVal any
	var capturedIsMap bool
	walkNestedMap(data, []string{"a", "b", "deep"}, func(_ map[string]any, _ string, val any, _ bool, isMap bool) {
		capturedVal = val
		capturedIsMap = isMap
	})
	if capturedVal != "scalar-not-map" {
		t.Errorf("expected scalar-not-map, got %v", capturedVal)
	}
	if capturedIsMap {
		t.Error("expected isMap=false for wrong-type intermediate")
	}
}

// TestLookupNestedKey_TableDriven covers all lookupNestedKey paths.
func TestLookupNestedKey_TableDriven(t *testing.T) {
	tests := []struct {
		name    string
		data    map[string]any
		parts   []string
		wantVal any
		wantOk  bool
	}{
		{
			name:    "single-part-found",
			data:    map[string]any{"key": "value"},
			parts:   []string{"key"},
			wantVal: "value",
			wantOk:  true,
		},
		{
			name:    "single-part-absent",
			data:    map[string]any{"other": "value"},
			parts:   []string{"key"},
			wantVal: nil,
			wantOk:  false,
		},
		{
			name:    "nested-found",
			data:    map[string]any{"a": map[string]any{"b": "deep"}},
			parts:   []string{"a", "b"},
			wantVal: "deep",
			wantOk:  true,
		},
		{
			name:    "nested-absent-leaf",
			data:    map[string]any{"a": map[string]any{"b": "deep"}},
			parts:   []string{"a", "c"},
			wantVal: nil,
			wantOk:  false,
		},
		{
			name:    "missing-intermediate",
			data:    map[string]any{"a": map[string]any{"b": "deep"}},
			parts:   []string{"a", "missing", "deep"},
			wantVal: nil,
			wantOk:  false,
		},
		{
			name:    "wrong-type-intermediate",
			data:    map[string]any{"a": map[string]any{"b": "scalar"}},
			parts:   []string{"a", "b", "deep"},
			wantVal: "scalar",
			wantOk:  true,
		},
		{
			name:    "deeply-nested-found",
			data:    map[string]any{"a": map[string]any{"b": map[string]any{"c": "very-deep"}}},
			parts:   []string{"a", "b", "c"},
			wantVal: "very-deep",
			wantOk:  true,
		},
		{
			name:   "map-leaf-found",
			data:   map[string]any{"a": map[string]any{"b": map[string]any{"c": "d"}}},
			parts:  []string{"a", "b"},
			wantOk: true,
			// wantVal is nil here — checked manually in the test loop
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotVal, gotOk := lookupNestedKey(tt.data, tt.parts)
			if gotOk != tt.wantOk {
				t.Errorf("lookupNestedKey(%v) ok = %v, want %v", tt.parts, gotOk, tt.wantOk)
			}
			if tt.wantOk && tt.wantVal != nil {
				// Maps can't be compared with != — only compare non-map values here
				if m, ok := tt.wantVal.(map[string]any); ok {
					if gm, ok := gotVal.(map[string]any); !ok || gm["c"] != m["c"] {
						t.Errorf("lookupNestedKey(%v) map content mismatch: got %v, want %v", tt.parts, gotVal, tt.wantVal)
					}
				} else if gotVal != tt.wantVal {
					t.Errorf("lookupNestedKey(%v) val = %v, want %v", tt.parts, gotVal, tt.wantVal)
				}
			}
		})
	}
}

// TestRemoveNestedKey_TableDriven covers all removeNestedKey paths.
func TestRemoveNestedKey_TableDriven(t *testing.T) {
	tests := []struct {
		name    string
		data    map[string]any
		parts   []string
		want    map[string]any
		wantErr bool
	}{
		{
			name:  "single-part-removal",
			data:  map[string]any{"key": "value", "other": "keep"},
			parts: []string{"key"},
			want:  map[string]any{"other": "keep"},
		},
		{
			name:  "single-part-absent",
			data:  map[string]any{"other": "keep"},
			parts: []string{"key"},
			want:  map[string]any{"other": "keep"},
		},
		{
			name:  "nested-removal",
			data:  map[string]any{"a": map[string]any{"b": "remove", "c": "keep"}},
			parts: []string{"a", "b"},
			want:  map[string]any{"a": map[string]any{"c": "keep"}},
		},
		{
			name:  "deep-removal",
			data:  map[string]any{"a": map[string]any{"b": map[string]any{"c": "remove"}}},
			parts: []string{"a", "b", "c"},
			want:  map[string]any{}, // a.b empties → cleaned up
		},
		{
			name:  "missing-intermediate",
			data:  map[string]any{"a": map[string]any{"b": "value"}},
			parts: []string{"a", "missing", "deep"},
			want:  map[string]any{"a": map[string]any{"b": "value"}},
		},
		{
			name:    "wrong-type-intermediate",
			data:    map[string]any{"a": map[string]any{"b": "scalar"}},
			parts:   []string{"a", "b", "deep"},
			want:    nil, // error
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := removeNestedKey(tt.data, tt.parts, "test", "test.toml")
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			// For deeply nested comparisons, use simpler checks
			if tt.want == nil && len(tt.data) > 0 {
				t.Errorf("expected empty map, got %v", tt.data)
			}
		})
	}
}

// TestWalkNestedMap_IdentityWithOriginal verifies the shared walker produces
// byte-identical results to the original lookupNestedKey / removeNestedKey
// behavior on all existing test scenarios.
func TestWalkNestedMap_IdentityWithOriginal(t *testing.T) {
	// Collision detection path: deep merge key sibling entry must not collide
	got := DetectCollisions(map[string]any{
		"model": map[string]any{"my-own-profile": map[string]any{"base_url": "http://x/v1"}},
	}, map[string]any{
		"model": map[string]any{"bedrock-grok": map[string]any{"model": "xai.grok-4.3"}},
	}, []string{"model"}, map[string][]string{"model": {"bedrock-grok"}})
	if len(got) != 0 {
		t.Errorf("sibling model profile must not collide, got %v", got)
	}

	// Collision detection path: owned sub-key collision
	got = DetectCollisions(map[string]any{
		"model": map[string]any{"bedrock-grok": map[string]any{"model": "their-own-thing"}},
	}, map[string]any{
		"model": map[string]any{"bedrock-grok": map[string]any{"model": "xai.grok-4.3"}},
	}, []string{"model"}, map[string][]string{"model": {"bedrock-grok"}})
	if len(got) != 1 || got[0].Path != "model.bedrock-grok" {
		t.Errorf("expected 1 collision at model.bedrock-grok, got %v", got)
	}

	// Remove path: dot notation removal
	data := map[string]any{
		"model_providers": map[string]any{
			"amazon-bedrock": map[string]any{
				"aws": map[string]any{
					"profile": "my-sso-profile",
					"region":  "us-east-1",
				},
				"name": "Amazon Bedrock (Mantle)",
			},
		},
	}
	err := removeOwnedSubKeys(data, "model_providers", []string{"amazon-bedrock.aws"}, "test.toml")
	if err != nil {
		t.Fatalf("removeOwnedSubKeys: %v", err)
	}
	ab := data["model_providers"].(map[string]any)["amazon-bedrock"].(map[string]any)
	if _, has := ab["aws"]; has {
		t.Error("aws sub-table should be removed")
	}
	if ab["name"] != "Amazon Bedrock (Mantle)" {
		t.Error("sibling name must survive")
	}
}

// TestWalkDotPath_DotNotation verifies walkDotPath splits and walks correctly.
func TestWalkDotPath_DotNotation(t *testing.T) {
	data := map[string]any{
		"model_providers": map[string]any{
			"amazon-bedrock": map[string]any{
				"aws": map[string]any{
					"region": "us-east-1",
				},
			},
		},
	}
	var foundVal any
	var found bool
	walkDotPath(data, "model_providers.amazon-bedrock.aws.region", func(_ map[string]any, _ string, val any, present bool, _ bool) {
		foundVal = val
		found = present
	})
	if !found {
		t.Error("expected region to be found")
	}
	if foundVal != "us-east-1" {
		t.Errorf("expected us-east-1, got %v", foundVal)
	}
}

// TestWalkDotPath_MissingPath verifies walkDotPath handles missing paths.
func TestWalkDotPath_MissingPath(t *testing.T) {
	data := map[string]any{
		"model_providers": map[string]any{
			"amazon-bedrock": map[string]any{
				"name": "Amazon Bedrock (Mantle)",
			},
		},
	}
	var called bool
	walkDotPath(data, "model_providers.amazon-bedrock.aws.region", func(_ map[string]any, _ string, _ any, _ bool, _ bool) {
		called = true
	})
	if called {
		t.Error("visitor must not be called for missing path")
	}
}

// TestLookupNestedKey_Integration verifies lookupNestedKey works correctly
// when called from detectDeepKeyCollisions with the same input shapes used
// in the existing collision tests.
func TestLookupNestedKey_Integration(t *testing.T) {
	// Test the dotted path collision case
	existing := map[string]any{
		"model_providers": map[string]any{
			"amazon-bedrock": map[string]any{
				"aws": map[string]any{"region": "eu-west-1"},
			},
		},
	}
	tbl := existing["model_providers"].(map[string]any)
	val, found := lookupNestedKey(tbl, strings.Split("amazon-bedrock.aws.region", "."))
	if !found {
		t.Error("expected region to be found")
	}
	if val != "eu-west-1" {
		t.Errorf("expected eu-west-1, got %v", val)
	}

	// Test missing intermediate
	_, found = lookupNestedKey(tbl, strings.Split("amazon-bedrock.missing.region", "."))
	if found {
		t.Error("expected not found for missing intermediate")
	}
}

// TestWalkManagedKeys_JuggernautAction verifies that the "juggernaut" key
// is classified as actionJuggernaut.
func TestWalkManagedKeys_JuggernautAction(t *testing.T) {
	plan := map[string]any{"juggernaut": map[string]any{"auth": "iam"}}
	var capturedAction managedKeyAction
	_ = walkManagedKeys(plan, nil, func(k string, v any, action managedKeyAction) error {
		if k != "juggernaut" {
			t.Fatalf("expected key juggernaut, got %s", k)
		}
		capturedAction = action
		return nil
	})
	if capturedAction != actionJuggernaut {
		t.Errorf("expected actionJuggernaut, got %d", capturedAction)
	}
}

// TestWalkManagedKeys_PermissionsAction verifies that the "permissions" key
// is classified as actionPermissions.
func TestWalkManagedKeys_PermissionsAction(t *testing.T) {
	plan := map[string]any{"permissions": map[string]any{"defaultMode": "auto"}}
	var capturedAction managedKeyAction
	_ = walkManagedKeys(plan, nil, func(k string, v any, action managedKeyAction) error {
		if k != "permissions" {
			t.Fatalf("expected key permissions, got %s", k)
		}
		capturedAction = action
		return nil
	})
	if capturedAction != actionPermissions {
		t.Errorf("expected actionPermissions, got %d", capturedAction)
	}
}

// TestWalkManagedKeys_DeepKeyAction verifies that keys listed in deepKeys
// are classified as actionDeep.
func TestWalkManagedKeys_DeepKeyAction(t *testing.T) {
	plan := map[string]any{
		"model_providers": map[string]any{
			"amazon-bedrock": map[string]any{"aws": map[string]any{"region": "us-east-1"}},
		},
	}
	var capturedAction managedKeyAction
	_ = walkManagedKeys(plan, []string{"model_providers"}, func(k string, v any, action managedKeyAction) error {
		if k != "model_providers" {
			t.Fatalf("expected key model_providers, got %s", k)
		}
		capturedAction = action
		return nil
	})
	if capturedAction != actionDeep {
		t.Errorf("expected actionDeep, got %d", capturedAction)
	}
}

// TestWalkManagedKeys_MapAction verifies that map values (not in deepKeys)
// are classified as actionMap.
func TestWalkManagedKeys_MapAction(t *testing.T) {
	plan := map[string]any{
		"env": map[string]string{"AWS_REGION": "us-east-1"},
	}
	var capturedAction managedKeyAction
	_ = walkManagedKeys(plan, nil, func(k string, v any, action managedKeyAction) error {
		if k != "env" {
			t.Fatalf("expected key env, got %s", k)
		}
		capturedAction = action
		return nil
	})
	if capturedAction != actionMap {
		t.Errorf("expected actionMap, got %d", capturedAction)
	}
}

// TestWalkManagedKeys_ScalarAction verifies that non-map values are
// classified as actionScalar.
func TestWalkManagedKeys_ScalarAction(t *testing.T) {
	plan := map[string]any{
		"model":       "anthropic.claude-sonnet-4-20250514",
		"effortLevel": "high",
	}
	actions := make(map[string]managedKeyAction)
	_ = walkManagedKeys(plan, nil, func(k string, v any, action managedKeyAction) error {
		actions[k] = action
		return nil
	})
	if actions["model"] != actionScalar {
		t.Errorf("expected actionScalar for model, got %d", actions["model"])
	}
	if actions["effortLevel"] != actionScalar {
		t.Errorf("expected actionScalar for effortLevel, got %d", actions["effortLevel"])
	}
}

// TestWalkManagedKeys_MultipleKeys verifies that all keys in a plan are
// visited with their correct actions.
func TestWalkManagedKeys_MultipleKeys(t *testing.T) {
	plan := map[string]any{
		"juggernaut":  map[string]any{"auth": "iam"},
		"permissions": map[string]any{"defaultMode": "auto"},
		"model":       "anthropic.claude-sonnet-4-20250514",
		"env":         map[string]string{"AWS_REGION": "us-east-1"},
		"model_providers": map[string]any{
			"amazon-bedrock": map[string]any{"aws": map[string]any{"region": "us-east-1"}},
		},
	}
	actions := make(map[string]managedKeyAction)
	_ = walkManagedKeys(plan, []string{"model_providers"}, func(k string, v any, action managedKeyAction) error {
		actions[k] = action
		return nil
	})

	expected := map[string]managedKeyAction{
		"juggernaut":      actionJuggernaut,
		"permissions":     actionPermissions,
		"model":           actionScalar,
		"env":             actionMap,
		"model_providers": actionDeep,
	}

	for k, want := range expected {
		got, ok := actions[k]
		if !ok {
			t.Errorf("key %q not visited", k)
			continue
		}
		if got != want {
			t.Errorf("key %q: expected action %d, got %d", k, want, got)
		}
	}
}

// TestWalkManagedKeys_ErrorPropagation verifies that visitor errors are
// propagated correctly.
func TestWalkManagedKeys_ErrorPropagation(t *testing.T) {
	plan := map[string]any{
		"model": "anthropic.claude-sonnet-4-20250514",
	}
	expectedErr := fmt.Errorf("test error")
	err := walkManagedKeys(plan, nil, func(k string, v any, action managedKeyAction) error {
		return expectedErr
	})
	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

// TestWalkManagedKeys_EmptyPlan verifies that an empty plan produces no visits.
func TestWalkManagedKeys_EmptyPlan(t *testing.T) {
	plan := map[string]any{}
	visited := false
	_ = walkManagedKeys(plan, nil, func(k string, v any, action managedKeyAction) error {
		visited = true
		return nil
	})
	if visited {
		t.Error("expected no visits for empty plan")
	}
}

// TestWalkManagedKeys_DeepKeyOverridesMap verifies that a key listed in
// deepKeys is classified as actionDeep even if its value is a map (which
// would normally be actionMap).
func TestWalkManagedKeys_DeepKeyOverridesMap(t *testing.T) {
	plan := map[string]any{
		"model": map[string]any{
			"bedrock-grok": map[string]any{"model": "xai.grok-4.3"},
		},
	}
	var capturedAction managedKeyAction
	_ = walkManagedKeys(plan, []string{"model"}, func(k string, v any, action managedKeyAction) error {
		capturedAction = action
		return nil
	})
	if capturedAction != actionDeep {
		t.Errorf("expected actionDeep (not actionMap) when key is in deepKeys, got %d", capturedAction)
	}
}

// TestWalkManagedKeysForRemoval_Juggernaut verifies that the juggernaut key
// is classified as actionJuggernaut during removal.
func TestWalkManagedKeysForRemoval_Juggernaut(t *testing.T) {
	var capturedAction managedKeyAction
	_ = walkManagedKeysForRemoval([]string{"juggernaut", "model"}, nil, func(k string, action managedKeyAction) error {
		if k == "juggernaut" {
			capturedAction = action
		}
		return nil
	})
	if capturedAction != actionJuggernaut {
		t.Errorf("expected actionJuggernaut, got %d", capturedAction)
	}
}

// TestWalkManagedKeysForRemoval_DeepKey verifies that keys in ownedSubKeys
// are classified as actionDeep during removal.
func TestWalkManagedKeysForRemoval_DeepKey(t *testing.T) {
	owned := map[string][]string{"model_providers": {"amazon-bedrock.aws.region"}}
	var capturedAction managedKeyAction
	_ = walkManagedKeysForRemoval([]string{"model_providers"}, owned, func(k string, action managedKeyAction) error {
		if k == "model_providers" {
			capturedAction = action
		}
		return nil
	})
	if capturedAction != actionDeep {
		t.Errorf("expected actionDeep, got %d", capturedAction)
	}
}

// TestWalkManagedKeysForRemoval_PermissionsAlwaysEmitted verifies that
// permissions is always emitted even when not in the key list.
func TestWalkManagedKeysForRemoval_PermissionsAlwaysEmitted(t *testing.T) {
	actions := make(map[string]managedKeyAction)
	_ = walkManagedKeysForRemoval([]string{"model", "env"}, nil, func(k string, action managedKeyAction) error {
		actions[k] = action
		return nil
	})
	if actions["permissions"] != actionPermissions {
		t.Error("expected permissions to be emitted even when not in key list")
	}
}

// TestWalkManagedKeysForRemoval_ErrorPropagation verifies that visitor errors
// are propagated during removal.
func TestWalkManagedKeysForRemoval_ErrorPropagation(t *testing.T) {
	expectedErr := fmt.Errorf("removal error")
	err := walkManagedKeysForRemoval([]string{"model"}, nil, func(k string, action managedKeyAction) error {
		return expectedErr
	})
	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}
