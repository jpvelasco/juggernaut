package activation

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

// TestRemoveBlock_OrphanBeginPreservesTrailingContent guards against the
// data-loss bug where a BEGIN marker without a matching END caused every
// following line to be dropped.
func TestRemoveBlock_OrphanBeginPreservesTrailingContent(t *testing.T) {
	content := strings.Join([]string{
		"export FOO=bar",
		BeginMarker,
		"export KEEP_ME=1",
		"export PATH=/custom:$PATH",
	}, "\n")

	out, found := removeBlock(content)
	if found {
		t.Error("orphaned begin marker should not report a removed block")
	}
	for _, kept := range []string{"export FOO=bar", "export KEEP_ME=1", "export PATH=/custom:$PATH"} {
		if !strings.Contains(out, kept) {
			t.Errorf("expected %q to be preserved after an orphaned begin marker, output:\n%s", kept, out)
		}
	}
}

// TestRemoveBlock_MatchedPairRemoved confirms a well-formed block is still
// stripped and surrounding content preserved.
func TestRemoveBlock_MatchedPairRemoved(t *testing.T) {
	content := strings.Join([]string{
		"keep-top",
		BeginMarker,
		"claude() { juggernaut launch -- \"$@\"; }",
		EndMarker,
		"keep-bottom",
	}, "\n")

	out, found := removeBlock(content)
	if !found {
		t.Fatal("expected matched block to be reported as removed")
	}
	if strings.Contains(out, BeginMarker) || strings.Contains(out, "juggernaut launch") {
		t.Errorf("block body should be removed, output:\n%s", out)
	}
	if !strings.Contains(out, "keep-top") || !strings.Contains(out, "keep-bottom") {
		t.Errorf("surrounding content must be preserved, output:\n%s", out)
	}
}

// TestRemoveBlock_MultipleBlocks confirms every matched block is removed while
// interleaved user content survives.
func TestRemoveBlock_MultipleBlocks(t *testing.T) {
	content := strings.Join([]string{
		"keep-1",
		BeginMarker,
		"block-a",
		EndMarker,
		"keep-2",
		BeginMarker,
		"block-b",
		EndMarker,
		"keep-3",
	}, "\n")

	out, found := removeBlock(content)
	if !found {
		t.Fatal("expected removal to report true")
	}
	for _, gone := range []string{"block-a", "block-b", BeginMarker} {
		if strings.Contains(out, gone) {
			t.Errorf("expected %q to be removed, output:\n%s", gone, out)
		}
	}
	for _, kept := range []string{"keep-1", "keep-2", "keep-3"} {
		if !strings.Contains(out, kept) {
			t.Errorf("expected %q to be preserved, output:\n%s", kept, out)
		}
	}
}

// TestUpsertBlock_OrphanBeginPreservesTrailingContent proves the install path
// (upsertBlock -> removeBlock) does not destroy user content when a profile
// contains an orphaned begin marker.
func TestUpsertBlock_OrphanBeginPreservesTrailingContent(t *testing.T) {
	content := "export KEEP_ME=1\n" + BeginMarker + "\nexport AFTER=2\n"

	out := upsertBlock(content, "claude() { :; }")
	if !strings.Contains(out, "export KEEP_ME=1") {
		t.Errorf("content before the orphaned marker must be preserved, output:\n%s", out)
	}
	if !strings.Contains(out, "export AFTER=2") {
		t.Errorf("content after the orphaned marker must be preserved, output:\n%s", out)
	}
}

// TestRemoveTarget_OrphanBeginPreservesTrailingContent exercises the public
// uninstall path end-to-end against a real (temp) profile file.
func TestRemoveTarget_OrphanBeginPreservesTrailingContent(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".bashrc")
	content := "export KEEP=1\n" + BeginMarker + "\nexport AFTER=2\nalias x='y'\n"
	if err := safepath.WriteFile(home, path, []byte(content)); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	// The orphaned marker is not a complete block, so nothing is removed and
	// the file is left intact.
	if _, err := RemoveTarget(path); err != nil {
		t.Fatalf("RemoveTarget() error: %v", err)
	}

	raw, err := safepath.ReadFile(home, path)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	got := string(raw)
	for _, kept := range []string{"export KEEP=1", "export AFTER=2", "alias x='y'"} {
		if !strings.Contains(got, kept) {
			t.Errorf("expected %q to survive uninstall over an orphaned marker, content:\n%s", kept, got)
		}
	}
}
