package config

import (
	"fmt"
	"strings"
)

// walkNestedMapVisitor is called by walkNestedMap at the resolved leaf of a
// dotted path. parent is the map containing the leaf, key is the leaf key,
// val is the resolved value (nil if absent), present indicates whether the key
// exists, and isMap indicates whether the value is a map[string]any.
type walkNestedMapVisitor func(parent map[string]any, key string, val any, present bool, isMap bool)

// walkNestedMap traverses a dotted path through nested maps and invokes the
// visitor at the resolved leaf. It returns immediately if an intermediate key
// is missing (nothing to traverse). If an intermediate exists but is not a map,
// the visitor is called at that point with present=true, isMap=false.
func walkNestedMap(root map[string]any, parts []string, visit walkNestedMapVisitor) {
	if len(parts) == 1 {
		val, present := root[parts[0]]
		_, isMap := val.(map[string]any)
		visit(root, parts[0], val, present, isMap)
		return
	}

	key := parts[0]
	val, present := root[key]
	if !present {
		return // missing intermediate — nothing to traverse
	}
	if child, ok := val.(map[string]any); ok {
		walkNestedMap(child, parts[1:], visit)
		return
	}
	// Present but not a map — report the type mismatch.
	visit(root, key, val, true, false)
}

// lookupNestedKey walks a slice of dotted path parts through nested maps,
// returning the leaf value if the path resolves to a present value. A
// genuinely MISSING intermediate is "nothing to collide with" (consistent
// with removeNestedKey's missing-intermediate no-op) — but an intermediate
// that is PRESENT with the wrong type (not a map) is not "missing", it's a
// foreign value incompatible with the table Juggernaut expects to write
// through; mergeNestedPrefix's recursion guard would silently let Juggernaut's
// map replace that scalar with no error, so it must be surfaced as a
// collision at that intermediate's own path rather than treated as absent.
func lookupNestedKey(tbl map[string]any, parts []string) (any, bool) {
	var result any
	var found bool
	walkNestedMap(tbl, parts, func(_ map[string]any, _ string, val any, present bool, isMap bool) {
		result = val
		// present and isMap: normal leaf found
		// present and !isMap: wrong-type intermediate — collision, not "missing"
		found = present
	})
	return result, found
}

// removeNestedKey walks a slice of path parts through nested maps and deletes
// the leaf key. Empty parent maps are cleaned up on the way back so orphan
// tables don't remain. The base case (single-part path) delegates to
// walkNestedMap; multi-part paths recurse with the same empty-parent cleanup.
func removeNestedKey(tbl map[string]any, parts []string, prefix string, path string) error {
	if len(parts) == 1 {
		walkNestedMap(tbl, parts, func(parent map[string]any, key string, _ any, present bool, _ bool) {
			if present {
				delete(parent, key)
			}
		})
		return nil
	}

	key := parts[0]
	val, ok := tbl[key]
	if !ok {
		return nil // sub-key doesn't exist — nothing to remove
	}
	child, isMap := val.(map[string]any)
	if !isMap {
		return fmt.Errorf("cannot remove nested key %q in %s: expected a table at %s but found %T",
			prefix, path, key, val)
	}
	if err := removeNestedKey(child, parts[1:], prefix, path); err != nil {
		return err
	}
	// Clean up empty parent maps.
	if len(child) == 0 {
		delete(tbl, key)
	}
	return nil
}

// walkDotPath splits a dot-notation path and walks it through a nested map,
// calling the visitor at each resolved level. Used by removeOwnedSubKeys and
// detectDeepKeyCollisions to traverse the same paths.
func walkDotPath(root map[string]any, dotPath string, visit walkNestedMapVisitor) {
	parts := strings.Split(dotPath, ".")
	walkNestedMap(root, parts, visit)
}

// managedKeyAction represents the disposition of a key during a managed-key walk.
type managedKeyAction int

const (
	actionJuggernaut  managedKeyAction = iota // "juggernaut" block — always fully owned
	actionPermissions                         // "permissions" — special handler
	actionDeep                                // deep-merge key — sub-key walk
	actionMap                                 // map value — sub-key iteration
	actionScalar                              // scalar — whole value
)

// walkManagedKeyVisitor is called for each key in the plan with its action type.
type walkManagedKeyVisitor func(key string, value any, action managedKeyAction) error

// walkManagedKeys iterates over the plan keys and dispatches each to the
// visitor with the appropriate action. It pre-computes the deepKeys set and
// classifies each key before calling the visitor.
func walkManagedKeys(plan map[string]any, deepKeys []string, visit walkManagedKeyVisitor) error {
	deep := make(map[string]bool, len(deepKeys))
	for _, k := range deepKeys {
		deep[k] = true
	}
	for k, v := range plan {
		switch {
		case k == "juggernaut":
			if err := visit(k, v, actionJuggernaut); err != nil {
				return err
			}
		case k == "permissions":
			if err := visit(k, v, actionPermissions); err != nil {
				return err
			}
		case deep[k]:
			if err := visit(k, v, actionDeep); err != nil {
				return err
			}
		case isStringKeyedMap(v):
			if err := visit(k, v, actionMap); err != nil {
				return err
			}
		default:
			if err := visit(k, v, actionScalar); err != nil {
				return err
			}
		}
	}
	return nil
}

// walkManagedKeyRemovalVisitor is called for each key during removal, carrying
// the key name and its classified action. Unlike walkManagedKeyVisitor, this
// does not carry a value — removal only needs the key name and classification.
type walkManagedKeyRemovalVisitor func(key string, action managedKeyAction) error

// walkManagedKeysForRemoval is the removal counterpart of walkManagedKeys.
// It classifies a list of key names (without values) and dispatches each to
// the visitor. The permissions key is always emitted so RemoveManagedKeysDeep
// can ensure the Juggernaut-managed permissions sub-key is stripped even when
// "permissions" was not in the key list (matches legacy behavior).
func walkManagedKeysForRemoval(keys []string, ownedSubKeys map[string][]string, visit walkManagedKeyRemovalVisitor) error {
	deep := make(map[string]bool, len(ownedSubKeys))
	for k := range ownedSubKeys {
		deep[k] = true
	}
	seen := make(map[string]bool, len(keys)+1)
	for _, k := range keys {
		seen[k] = true
		var action managedKeyAction
		switch {
		case k == "juggernaut":
			action = actionJuggernaut
		case k == "permissions":
			action = actionPermissions
		case deep[k]:
			action = actionDeep
		default:
			action = actionScalar
		}
		if err := visit(k, action); err != nil {
			return err
		}
	}
	// Always emit permissions so the managed sub-key is stripped even if
	// "permissions" was not in the key list (matches legacy behavior).
	if !seen["permissions"] {
		if err := visit("permissions", actionPermissions); err != nil {
			return err
		}
	}
	return nil
}

// classifyManagedKey returns the action for a key name during removal.
// Unlike walkManagedKeys, this does not need the value — removal only cares
// about the key's classification (juggernaut, permissions, deep, or scalar).
func classifyManagedKey(k string, deepKeys []string) managedKeyAction {
	deep := make(map[string]bool, len(deepKeys))
	for _, dk := range deepKeys {
		deep[dk] = true
	}
	switch {
	case k == "juggernaut":
		return actionJuggernaut
	case k == "permissions":
		return actionPermissions
	case deep[k]:
		return actionDeep
	default:
		return actionScalar
	}
}
