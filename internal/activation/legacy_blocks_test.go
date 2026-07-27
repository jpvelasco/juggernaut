package activation

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
	"github.com/jpvelasco/juggernaut/v5/internal/testutil"
)

func TestHasLegacyLauncherBlock(t *testing.T) {
	with := LegacyLauncherBegin + "\nclaude() { ... }\n" + LegacyLauncherEnd
	if !HasLegacyLauncherBlock(with) {
		t.Error("expected legacy launcher block to be detected")
	}
	if HasLegacyLauncherBlock("nothing here") {
		t.Error("did not expect detection in unrelated content")
	}
	// Orphaned begin marker only — not a complete block.
	if HasLegacyLauncherBlock(LegacyLauncherBegin + "\n") {
		t.Error("orphaned begin marker should not count as a block")
	}
}

func TestHasLegacyBedrockBlock(t *testing.T) {
	with := LegacyBedrockBegin + "\nexport X=1\n" + LegacyBedrockEnd
	if !HasLegacyBedrockBlock(with) {
		t.Error("expected legacy bedrock block to be detected")
	}
	if HasLegacyBedrockBlock("unrelated") {
		t.Error("did not expect detection in unrelated content")
	}
}

func TestRemoveBlockWithMarkers_Multiple(t *testing.T) {
	content := strings.Join([]string{
		"keep-top",
		LegacyLauncherBegin,
		"block-a",
		LegacyLauncherEnd,
		"keep-middle",
		LegacyLauncherBegin,
		"block-b",
		LegacyLauncherEnd,
		"keep-bottom",
	}, "\n")

	out, removed := removeBlockWithMarkers(content, LegacyLauncherBegin, LegacyLauncherEnd)
	if !removed {
		t.Fatal("expected removal to report true")
	}
	for _, gone := range []string{"block-a", "block-b", LegacyLauncherBegin} {
		if strings.Contains(out, gone) {
			t.Errorf("expected %q to be removed, output:\n%s", gone, out)
		}
	}
	for _, kept := range []string{"keep-top", "keep-middle", "keep-bottom"} {
		if !strings.Contains(out, kept) {
			t.Errorf("expected %q to be preserved, output:\n%s", kept, out)
		}
	}
}

func TestRemoveBlockWithMarkers_OrphanBeginUntouched(t *testing.T) {
	content := "keep\n" + LegacyLauncherBegin + "\ndangling\n"
	out, removed := removeBlockWithMarkers(content, LegacyLauncherBegin, LegacyLauncherEnd)
	if removed {
		t.Error("orphan begin marker should not be removed")
	}
	if !strings.Contains(out, "dangling") {
		t.Error("content after an orphan begin marker must be preserved")
	}
}

// TestRemoveTargetWithLegacy_RemovesLegacyBedrockBlock exercises the legacy
// Bedrock-config removal path through the public RemoveTargetWithLegacy entry.
func TestRemoveTargetWithLegacy_RemovesLegacyBedrockBlock(t *testing.T) {
	home := testutil.NewTestHome(t)
	path := filepath.Join(home, ".bashrc")
	content := "export KEEP=1\n" +
		LegacyBedrockBegin + "\nexport CLAUDE_CODE_USE_BEDROCK=1\n" + LegacyBedrockEnd + "\n" +
		"alias x='y'\n"
	if err := safepath.WriteFile(home, path, []byte(content)); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	ok, err := RemoveTargetWithLegacy(path)
	if err != nil {
		t.Fatalf("RemoveTargetWithLegacy() error: %v", err)
	}
	if !ok {
		t.Fatal("expected removal of the legacy bedrock block")
	}

	raw, err := safepath.ReadFile(home, path)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	got := string(raw)
	if HasLegacyBedrockBlock(got) {
		t.Errorf("legacy bedrock block should be gone, content:\n%s", got)
	}
	if !strings.Contains(got, "export KEEP=1") || !strings.Contains(got, "alias x='y'") {
		t.Errorf("unrelated content must be preserved, content:\n%s", got)
	}
}
