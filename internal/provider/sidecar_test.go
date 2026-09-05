package provider

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
	"github.com/jpvelasco/juggernaut/v5/internal/config"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
	"github.com/jpvelasco/juggernaut/v5/internal/testutil"
)

func TestOpenCode_SidecarPaths(t *testing.T) {
	p, err := Get("opencode")
	if err != nil {
		t.Fatalf("Get(opencode): %v", err)
	}
	src, ok := p.(SidecarAuthSource)
	if !ok {
		t.Fatal("opencode must implement SidecarAuthSource")
	}
	home := testutil.NewTestHome(t)

	proj, err := src.SidecarPath(home, "project")
	if err != nil {
		t.Fatalf("SidecarPath(project): %v", err)
	}
	if filepath.Base(proj) != SidecarFilename {
		t.Errorf("project sidecar = %q, want basename %s", proj, SidecarFilename)
	}
	// The project sidecar sits next to ./opencode.json — i.e. the working dir.
	if filepath.Dir(proj) != "." {
		t.Errorf("project sidecar = %q, want <workdir>/%s", proj, SidecarFilename)
	}

	user, err := src.SidecarPath(home, "user")
	if err != nil {
		t.Fatalf("SidecarPath(user): %v", err)
	}
	wantUser := filepath.Join(home, filepath.FromSlash(".config/opencode/.juggernaut.json"))
	if user != wantUser {
		t.Errorf("user sidecar = %q, want %q", user, wantUser)
	}

	paths := src.SidecarPaths(home)
	if len(paths) != 2 {
		t.Fatalf("SidecarPaths = %v, want 2 entries", paths)
	}
	if paths[0] != proj || paths[1] != user {
		t.Errorf("SidecarPaths order = %v, want project then user", paths)
	}
}

// TestOpenCode_OtherProvidersHaveNoSidecar pins the extension boundary:
// only opencode keeps the block outside the vendor file.
func TestOpenCode_OtherProvidersHaveNoSidecar(t *testing.T) {
	for _, name := range []string{"claude", "codex", "grok"} {
		p, _ := Get(name)
		if HasSidecar(p) {
			t.Errorf("%s must not implement SidecarAuthSource", name)
		}
		if _, ok := p.(SidecarAuthSource); ok {
			t.Errorf("%s unexpectedly implements SidecarAuthSource", name)
		}
	}
	if !HasSidecar(mustGetProvider(t, "opencode")) {
		t.Error("opencode must implement SidecarAuthSource")
	}
}

func mustGetProvider(t *testing.T, name string) Provider {
	t.Helper()
	p, err := Get(name)
	if err != nil {
		t.Fatalf("Get(%q): %v", name, err)
	}
	return p
}

// TestSidecar_WriteReadRoundTrip covers the write/read/delete cycle in both
// scopes plus the foreign-content and missing-file negative cases.
func TestSidecar_WriteReadRoundTrip(t *testing.T) {
	home := testutil.NewTestHome(t)
	p := mustGetProvider(t, "opencode")
	src := p.(SidecarAuthSource)

	t.Run("user", func(t *testing.T) {
		opts := baseOpts()
		opts.Scope = "user"
		opts.AuthMode = authmode.BedrockAPIKey
		opts.Region = "eu-west-1"
		if err := WriteSidecar(p, home, opts); err != nil {
			t.Fatalf("WriteSidecar(user): %v", err)
		}
		path, err := src.SidecarPath(home, "user")
		if err != nil {
			t.Fatalf("SidecarPath: %v", err)
		}
		// Owner-only permissions (0o600), content wraps the block under "juggernaut".
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("sidecar missing: %v", err)
		}
		// POSIX: owner-only 0o600. Windows maps the mode to the default ACL
		// (rw for the owner SID), so only the owner-rw bits are guaranteed.
		var want fs.FileMode = 0o600
		if runtime.GOOS == "windows" {
			want = 0o666
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("sidecar perms = %o, want %o", got, want)
		}
		if block := ReadSidecarFile(path); block == nil {
			t.Fatal("ReadSidecarFile(user) = nil, want the block")
		}
		mode, ok := ReadSidecarAuthMode(p, home)
		if !ok || mode != authmode.BedrockAPIKey {
			t.Errorf("ReadSidecarAuthMode = %q, %v; want bedrock-api-key", mode, ok)
		}
		if err := RemoveSidecar(p, home, []string{"user"}); err != nil {
			t.Fatalf("RemoveSidecar(user): %v", err)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("sidecar should be gone after RemoveSidecar, stat err = %v", err)
		}
		// Removing a missing sidecar is a no-op.
		if err := RemoveSidecar(p, home, []string{"user"}); err != nil {
			t.Errorf("RemoveSidecar on missing file: %v", err)
		}
	})

	// The project sidecar lives in the working dir (./.juggernaut.json), so
	// chdir into a scratch dir to keep the repo clean.
	workDir := t.TempDir()
	t.Chdir(workDir)
	t.Run("project", func(t *testing.T) {
		opts := baseOpts()
		opts.Scope = "project"
		opts.AuthMode = authmode.IAM
		if err := WriteSidecar(p, home, opts); err != nil {
			t.Fatalf("WriteSidecar(project): %v", err)
		}
		path, err := src.SidecarPath(home, "project")
		if err != nil {
			t.Fatalf("SidecarPath: %v", err)
		}
		want := filepath.Join(workDir, SidecarFilename)
		if !samePath(path, want) {
			t.Errorf("project sidecar = %q, want %q", path, want)
		}
		mode, ok := ReadSidecarAuthMode(p, home)
		if !ok || mode != authmode.IAM {
			t.Errorf("ReadSidecarAuthMode(project) = %q, %v; want iam", mode, ok)
		}
		if err := RemoveSidecar(p, home, []string{"project"}); err != nil {
			t.Fatalf("RemoveSidecar(project): %v", err)
		}
		if _, err := os.Stat(want); !os.IsNotExist(err) {
			t.Errorf("project sidecar should be gone, stat err = %v", err)
		}
	})
}

func samePath(a, b string) bool {
	ae, _ := filepath.Abs(a)
	be, _ := filepath.Abs(b)
	return strings.EqualFold(ae, be)
}

// TestSidecar_ReadForeignAndMissing: a user-owned .juggernaut.json (no
// managedBy gate) must not be misread, and a missing file must return
// ok=false — never an error.
func TestSidecar_ReadForeignAndMissing(t *testing.T) {
	home := testutil.NewTestHome(t)
	p := mustGetProvider(t, "opencode")
	src := p.(SidecarAuthSource)

	userPath, _ := src.SidecarPath(home, "user")
	if err := safepath.MkdirAll(filepath.Dir(userPath)); err != nil {
		t.Fatal(err)
	}
	// Missing file: no mode, no error.
	if mode, ok := ReadSidecarAuthMode(p, home); ok || mode != "" {
		t.Errorf("missing sidecar: got mode %q ok=%v, want empty/false", mode, ok)
	}
	// Foreign content with the same filename is never misread (managedBy gate).
	foreign := `{"juggernaut":{"auth":{"mode":"iam"},"meta":{"managedBy":"someone-else"}}}`
	if err := os.WriteFile(userPath, []byte(foreign), 0o600); err != nil {
		t.Fatal(err)
	}
	if mode, ok := ReadSidecarAuthMode(p, home); ok || mode != "" {
		t.Errorf("foreign sidecar: got mode %q ok=%v, want empty/false", mode, ok)
	}
	// Malformed JSON is silently skipped.
	if err := os.WriteFile(userPath, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if mode, ok := ReadSidecarAuthMode(p, home); ok || mode != "" {
		t.Errorf("malformed sidecar: got mode %q ok=%v, want empty/false", mode, ok)
	}
}

// TestSidecar_ProjectWinsOverUser pins launch precedence: a project-scope
// sidecar overrides the user-scope one, matching "read project then user".
func TestSidecar_ProjectWinsOverUser(t *testing.T) {
	home := testutil.NewTestHome(t)
	p := mustGetProvider(t, "opencode")
	src := p.(SidecarAuthSource)

	workDir := t.TempDir()
	t.Chdir(workDir)

	userPath, _ := src.SidecarPath(home, "user")
	if err := safepath.MkdirAll(filepath.Dir(userPath)); err != nil {
		t.Fatal(err)
	}
	userDoc := `{"juggernaut":{"auth":{"mode":"iam"},"meta":{"managedBy":"juggernaut"}}}`
	if err := os.WriteFile(userPath, []byte(userDoc), 0o600); err != nil {
		t.Fatal(err)
	}
	projPath, _ := src.SidecarPath(home, "project")
	projDoc := `{"juggernaut":{"auth":{"mode":"` + authmode.BedrockAPIKey + `"},"meta":{"managedBy":"juggernaut"}}}`
	if err := os.WriteFile(projPath, []byte(projDoc), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(projPath) })

	mode, ok := ReadSidecarAuthMode(p, home)
	if !ok || mode != authmode.BedrockAPIKey {
		t.Errorf("project sidecar must win over user: got %q, %v", mode, ok)
	}
}

// TestSidecar_NonSidecarProvidersNoOp: every helper must be a clean no-op for
// providers without the extension (the package helpers are what cmd/ calls).
func TestSidecar_NonSidecarProvidersNoOp(t *testing.T) {
	home := testutil.NewTestHome(t)
	opts := baseOpts()

	for _, name := range []string{"claude", "codex", "grok"} {
		p := mustGetProvider(t, name)
		if err := WriteSidecar(p, home, opts); err != nil {
			t.Errorf("%s: WriteSidecar must be a no-op, got %v", name, err)
		}
		if mode, ok := ReadSidecarAuthMode(p, home); ok || mode != "" {
			t.Errorf("%s: ReadSidecarAuthMode must be empty/false, got %q, %v", name, mode, ok)
		}
		if SidecarExists(p, home, "user") {
			t.Errorf("%s: SidecarExists must be false", name)
		}
		if err := RemoveSidecar(p, home, []string{"user", "project"}); err != nil {
			t.Errorf("%s: RemoveSidecar must be a no-op, got %v", name, err)
		}
	}
}

// TestMigrateSidecarLegacy pins the v6.2.0–v6.3.0 migration: a plain apply
// strips the in-file juggernaut block (only when managedBy is juggernaut) and
// the stale null/empty whitelist leaf, and returns a notice when it changed
// anything.
func TestMigrateSidecarLegacy(t *testing.T) {
	p := mustGetProvider(t, "opencode")

	t.Run("stripsManagedBlockAndStaleWhitelist", func(t *testing.T) {
		existing := map[string]any{
			"model": "amazon-bedrock/openai.gpt-oss-120b-1:0",
			"juggernaut": map[string]any{
				"auth": map[string]any{"mode": authmode.IAM, "region": "us-west-2"},
				"meta": map[string]any{"managedBy": "juggernaut"},
			},
			"provider": map[string]any{
				"amazon-bedrock": map[string]any{
					"options":   map[string]any{"region": "us-west-2"},
					"models":    map[string]any{},
					"whitelist": nil, // JSON null round-trips to nil
				},
			},
			"someUserKey": "keep",
		}
		notice := MigrateSidecarLegacy(p, existing)
		if notice == "" {
			t.Fatal("expected a migration notice")
		}
		if !strings.Contains(notice, "juggernaut") || !strings.Contains(notice, "whitelist") {
			t.Errorf("notice should mention both migrations, got %q", notice)
		}
		if _, ok := existing["juggernaut"]; ok {
			t.Error("legacy in-file juggernaut block must be removed")
		}
		ab, _ := existing["provider"].(map[string]any)["amazon-bedrock"].(map[string]any)
		if _, ok := ab["whitelist"]; ok {
			t.Error("stale null whitelist must be dropped")
		}
		if existing["someUserKey"] != "keep" {
			t.Error("user keys must survive migration")
		}
	})

	t.Run("preservesUserOwnedJuggernautKey", func(t *testing.T) {
		existing := map[string]any{
			"juggernaut": map[string]any{"auth": map[string]any{"mode": authmode.IAM}},
			// no meta.managedBy — a user key that merely shares the name
		}
		notice := MigrateSidecarLegacy(p, existing)
		if notice != "" {
			t.Errorf("user-owned juggernaut key must not be migrated, notice = %q", notice)
		}
		if _, ok := existing["juggernaut"]; !ok {
			t.Error("user-owned juggernaut key was deleted — data loss")
		}
	})

	t.Run("emptyWhitelistArray", func(t *testing.T) {
		existing := map[string]any{
			"provider": map[string]any{
				"amazon-bedrock": map[string]any{"whitelist": []any{}},
			},
		}
		notice := MigrateSidecarLegacy(p, existing)
		if notice == "" {
			t.Fatal("expected a notice for the empty whitelist")
		}
		ab, _ := existing["provider"].(map[string]any)["amazon-bedrock"].(map[string]any)
		if _, ok := ab["whitelist"]; ok {
			t.Error("empty-array whitelist must be dropped")
		}
	})

	t.Run("cleanConfigNoNotice", func(t *testing.T) {
		existing := map[string]any{
			"model":    "amazon-bedrock/openai.gpt-oss-120b-1:0",
			"provider": map[string]any{"amazon-bedrock": map[string]any{"options": map[string]any{}}},
		}
		if notice := MigrateSidecarLegacy(p, existing); notice != "" {
			t.Errorf("clean config must not report a migration, got %q", notice)
		}
	})

	t.Run("nonSidecarProviderNoOp", func(t *testing.T) {
		codex := mustGetProvider(t, "codex")
		existing := map[string]any{"juggernaut": map[string]any{}}
		if notice := MigrateSidecarLegacy(codex, existing); notice != "" {
			t.Errorf("codex must not be migrated, got %q", notice)
		}
		if _, ok := existing["juggernaut"]; !ok {
			t.Error("codex in-file block must be untouched")
		}
	})
}

// errPathSidecar is a SidecarAuthSource whose SidecarPath always fails. It
// pins that every helper propagates the path error instead of panicking or
// silently proceeding (SidecarExists/ReadSidecarAuthMode degrade to "absent").
// Embedding the Provider interface (unused here) lets it satisfy the Provider
// parameter types without a full implementation.
type errPathSidecar struct{ Provider }

func (errPathSidecar) SidecarPath(string, string) (string, error) {
	return "", fmt.Errorf("cannot resolve sidecar path")
}

func (errPathSidecar) SidecarPaths(string) []string { return nil }

func TestSidecar_ErrPathSidecar(t *testing.T) {
	home := testutil.NewTestHome(t)
	p := errPathSidecar{}
	if !HasSidecar(p) {
		t.Fatal("fixture must implement SidecarAuthSource")
	}
	if err := WriteSidecar(p, home, baseOpts()); err == nil {
		t.Error("WriteSidecar must propagate the SidecarPath error")
	}
	if SidecarExists(p, home, "user") {
		t.Error("SidecarExists must be false when the path cannot be resolved")
	}
	if mode, ok := ReadSidecarAuthMode(p, home); ok || mode != "" {
		t.Errorf("ReadSidecarAuthMode = %q, %v; want empty/false on path error", mode, ok)
	}
	if err := RemoveSidecar(p, home, []string{"user"}); err == nil {
		t.Error("RemoveSidecar must propagate the SidecarPath error")
	}
}

// TestBlockAuthMode_Malformed pins defensive parsing: a block missing auth,
// with a non-string mode, or with a nil auth map all yield "" (no panic).
func TestBlockAuthMode_Malformed(t *testing.T) {
	if got := BlockAuthMode(nil); got != "" {
		t.Errorf("BlockAuthMode(nil) = %q, want \"\"", got)
	}
	if got := BlockAuthMode(map[string]any{"auth": "not-a-map"}); got != "" {
		t.Errorf("BlockAuthMode(non-map auth) = %q, want \"\"", got)
	}
	if got := BlockAuthMode(map[string]any{"auth": map[string]any{"mode": 42}}); got != "" {
		t.Errorf("BlockAuthMode(non-string mode) = %q, want \"\"", got)
	}
	if got := BlockAuthMode(map[string]any{}); got != "" {
		t.Errorf("BlockAuthMode(no auth key) = %q, want \"\"", got)
	}
}

// TestSidecar_ExistsTracksFile covers the SidecarExists happy path: false
// before the sidecar exists, true after a write, false again after removal.
func TestSidecar_ExistsTracksFile(t *testing.T) {
	home := testutil.NewTestHome(t)
	p := mustGetProvider(t, "opencode")

	if SidecarExists(p, home, "user") {
		t.Fatal("SidecarExists = true before any write")
	}
	if err := WriteSidecar(p, home, baseOpts()); err != nil {
		t.Fatalf("WriteSidecar: %v", err)
	}
	if !SidecarExists(p, home, "user") {
		t.Error("SidecarExists = false after WriteSidecar, want true")
	}
	if err := RemoveSidecar(p, home, []string{"user"}); err != nil {
		t.Fatalf("RemoveSidecar: %v", err)
	}
	if SidecarExists(p, home, "user") {
		t.Error("SidecarExists = true after RemoveSidecar, want false")
	}
}

// TestMigrateSidecarLegacy_NilConfig pins the nil guard: a provider with no
// existing config reports nothing to migrate (not an error, not a panic).
func TestMigrateSidecarLegacy_NilConfig(t *testing.T) {
	p := mustGetProvider(t, "opencode")
	if notice := MigrateSidecarLegacy(p, nil); notice != "" {
		t.Errorf("MigrateSidecarLegacy(nil) = %q, want \"\"", notice)
	}
}

// TestOpenCode_SidecarBlockJSONShape asserts the on-disk sidecar shape end to
// end: the block is wrapped under "juggernaut" so config.ParseJuggernautBlock
// (the shared reader used by activation) parses it unchanged.
func TestOpenCode_SidecarBlockJSONShape(t *testing.T) {
	home := testutil.NewTestHome(t)
	p := mustGetProvider(t, "opencode")
	opts := baseOpts()
	opts.Scope = "user"
	opts.AuthMode = authmode.BedrockAPIKey
	opts.Region = "eu-central-1"
	if err := WriteSidecar(p, home, opts); err != nil {
		t.Fatalf("WriteSidecar: %v", err)
	}
	src := p.(SidecarAuthSource)
	path, _ := src.SidecarPath(home, "user")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading sidecar: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("sidecar must be valid JSON: %v", err)
	}
	jb, ok := config.ParseJuggernautBlock(doc)
	if !ok {
		t.Fatal("sidecar must parse as a Juggernaut block (managedBy gate)")
	}
	if jb.AuthMode != authmode.BedrockAPIKey || jb.Region != "eu-central-1" {
		t.Errorf("sidecar auth = %q/%q, want %s/eu-central-1", jb.AuthMode, jb.Region, authmode.BedrockAPIKey)
	}
}
