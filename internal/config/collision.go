package config

import (
	"sort"
	"strings"
)

// Collision is one leaf Juggernaut is about to write that already holds a
// foreign (non-Juggernaut) value in an existing config file.
type Collision struct {
	Path     string // dotted leaf path, e.g. "env.AWS_REGION", "model.bedrock-grok"
	Existing any    // the foreign value already there
}

// DetectCollisions walks the leaf paths that plan is about to write into
// existing and returns every leaf that already holds a foreign value.
// deepKeys/ownedSubKeys mirror Provider.DeepMergeKeys()/OwnedSubKeys() — the
// same metadata that drives MergeConfigPlanDeep/RemoveManagedKeysDeep — so any
// provider that declares its owned keys gets collision detection for free.
//
// Presence alone is a collision, even if the existing value equals what
// Juggernaut would write: this function never silently decides a leaf is
// "already fine" on the caller's behalf ("Juggernaut law" — either Juggernaut
// owns a leaf, or the user does, never both).
//
// Callers are expected to only invoke this when the config is NOT already
// owned by Juggernaut (Provider.OwnsConfig returns false) — a re-apply of a
// file Juggernaut already owns must proceed with zero new friction.
func DetectCollisions(existing, plan map[string]any, deepKeys []string, ownedSubKeys map[string][]string) []Collision {
	var collisions []Collision
	_ = walkManagedKeys(plan, deepKeys, func(k string, v any, action managedKeyAction) error {
		switch action {
		case actionJuggernaut:
			return nil // always fully owned by Juggernaut, never checked
		case actionPermissions:
			collisions = append(collisions, detectPermissionsCollision(existing[k])...)
		case actionDeep:
			collisions = append(collisions, detectDeepKeyCollisions(existing[k], k, ownedSubKeys[k])...)
		case actionMap:
			incomingMap := asStringKeyedMapUnwrap(v)
			ev, present := existing[k]
			collisions = append(collisions, detectMapKeyCollisions(ev, present, k, incomingMap)...)
		default:
			ev, present := existing[k]
			if present {
				collisions = append(collisions, Collision{Path: k, Existing: ev})
			}
		}
		return nil
	})

	sort.Slice(collisions, func(i, j int) bool { return collisions[i].Path < collisions[j].Path })
	return collisions
}

// detectPermissionsCollision checks only permissions.defaultMode, mirroring
// mergePermissions' special-casing — a user's own allow/deny/ask rules never
// collide. If "permissions" is present but not a map, mergePermissions'
// `existing["permissions"].(map[string]any)` assertion would silently yield
// nil and the foreign value would be clobbered with no error — that type
// mismatch must itself be reported as a collision, not swallowed.
func detectPermissionsCollision(existingVal any) []Collision {
	if existingVal == nil {
		return nil
	}
	perms, ok := existingVal.(map[string]any)
	if !ok {
		return []Collision{{Path: "permissions", Existing: existingVal}}
	}
	if mode, present := perms["defaultMode"]; present {
		return []Collision{{Path: "permissions.defaultMode", Existing: mode}}
	}
	return nil
}

// isStringKeyedMap reports whether v is a string-keyed map (map[string]any or
// map[string]string). Used by walkManagedKeys to classify keys.
func isStringKeyedMap(v any) bool {
	_, ok := asStringKeyedMap(v)
	return ok
}

// asStringKeyedMap normalizes any string-keyed map shape a provider's plan
// might use (map[string]any, or map[string]string like Claude's native
// env — see schema.NativeKeys.Env) into map[string]any for uniform handling.
func asStringKeyedMap(v any) (map[string]any, bool) {
	switch m := v.(type) {
	case map[string]any:
		return m, true
	case map[string]string:
		out := make(map[string]any, len(m))
		for k, sv := range m {
			out[k] = sv
		}
		return out, true
	default:
		return nil, false
	}
}

// asStringKeyedMapUnwrap is like asStringKeyedMap but panics if the value is
// not a string-keyed map. It is only called from walkManagedKeys when the
// actionMap branch has already confirmed the value is a map.
func asStringKeyedMapUnwrap(v any) map[string]any {
	m, ok := asStringKeyedMap(v)
	if !ok {
		panic("config: asStringKeyedMapUnwrap called with non-map value")
	}
	return m
}

// detectMapKeyCollisions handles a whole-value key whose incoming plan value
// is itself a map (e.g. Claude's env, modelOverrides): checked at the exact
// sub-key Juggernaut is about to write, not the whole map, so a user's
// unrelated sibling keys never collide. A type mismatch (existing holds a
// non-map) is itself a collision — the merge would fail the same way.
func detectMapKeyCollisions(existingVal any, present bool, key string, incoming map[string]any) []Collision {
	if !present {
		return nil
	}
	existingMap, ok := existingVal.(map[string]any)
	if !ok {
		return []Collision{{Path: key, Existing: existingVal}}
	}
	var collisions []Collision
	for sk := range incoming {
		if ev, ok := existingMap[sk]; ok {
			collisions = append(collisions, Collision{Path: key + "." + sk, Existing: ev})
		}
	}
	return collisions
}

// detectDeepKeyCollisions checks only the dotted owned sub-key paths (mirrors
// removeOwnedSubKeys' dot-notation walk), so a user's sibling entries in the
// same nested table never collide. If the key's root exists but is not a map,
// mergeNested would refuse the merge with a type-mismatch error anyway — but
// reporting it here up front gives the intended refusal message instead of a
// bare merge-time error.
func detectDeepKeyCollisions(existingVal any, key string, subs []string) []Collision {
	if existingVal == nil {
		return nil
	}
	tbl, ok := existingVal.(map[string]any)
	if !ok {
		return []Collision{{Path: key, Existing: existingVal}}
	}
	var collisions []Collision
	for _, sub := range subs {
		parts := strings.Split(sub, ".")
		if ev, found := lookupNestedKey(tbl, parts); found {
			collisions = append(collisions, Collision{Path: key + "." + sub, Existing: ev})
		}
	}
	return collisions
}

// lookupNestedKey is implemented in walker.go using the shared walkNestedMap helper.
// The declaration there uses the same signature and behavior as the original.
