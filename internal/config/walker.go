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
// tables don't remain.
func removeNestedKey(tbl map[string]any, parts []string, prefix string, path string) error {
	if len(parts) == 1 {
		delete(tbl, parts[0])
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
	} else {
		tbl[key] = child
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
