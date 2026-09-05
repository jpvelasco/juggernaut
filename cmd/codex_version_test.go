package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/doctor"
	"github.com/jpvelasco/juggernaut/v5/internal/provider"
)

// stubCodexProbe points the version source at a temp PATH dir holding a file
// named like the codex binary, and stubs the probe to report the given version.
// The real PATH resolution (activation.ResolveBinary) still runs; only the
// probe exec is faked (a stub file isn't a real executable on every OS). CI
// runners have no codex on PATH, so tests must not depend on the host binary.
// version "unknown" exercises the unparseable ("cannot determine") path.
func stubCodexProbe(t *testing.T, version string) {
	t.Helper()
	dir := t.TempDir()
	name := provider.MustGet("codex").BinaryNames()[0]
	if err := os.WriteFile(filepath.Join(dir, name), []byte("stub\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	restorePath := codexVersionPath
	codexVersionPath = dir
	restoreProbe := codexVersionProbe
	codexVersionProbe = func(string) (string, bool) { return version, true }
	t.Cleanup(func() {
		codexVersionPath = restorePath
		codexVersionProbe = restoreProbe
	})
}

// stubCodexAbsent points the version source at an empty PATH dir so the real
// resolution legitimately fails with exec.ErrNotFound.
func stubCodexAbsent(t *testing.T) {
	t.Helper()
	restorePath := codexVersionPath
	codexVersionPath = t.TempDir()
	t.Cleanup(func() { codexVersionPath = restorePath })
}

func TestParseCodexVersion(t *testing.T) {
	cases := []struct {
		out  string
		want string
		ok   bool
	}{
		{out: "codex-cli 0.153.4\n", want: "0.153.4", ok: true},
		{out: "codex-cli 0.148.0-alpha.9", want: "0.148.0-alpha.9", ok: true},
		{out: "0.153.4\n", want: "0.153.4", ok: true},     // bare version accepted
		{out: "codex-cli\n", want: "codex-cli", ok: true}, // single field: triple check rejects downstream
		{out: "\n", ok: false},                            // nothing
	}
	for _, c := range cases {
		got, ok := parseCodexVersion(c.out)
		if ok != c.ok || got != c.want {
			t.Errorf("parseCodexVersion(%q) = (%q, %v), want (%q, %v)", c.out, got, ok, c.want, c.ok)
		}
	}
}

func TestCodexVersionAtLeast(t *testing.T) {
	cases := []struct {
		v, min string
		want   bool
	}{
		{v: "0.153.4", min: "0.153.4", want: true}, // equal
		{v: "0.153.5", min: "0.153.4", want: true},
		{v: "0.154.0", min: "0.153.4", want: true},
		{v: "1.0.0", min: "0.153.4", want: true},
		{v: "0.153.3", min: "0.153.4", want: false},
		{v: "0.152.9", min: "0.153.4", want: false},
		{v: "0.148.0-alpha.9", min: "0.153.4", want: false}, // suffix stripped for compare
		{v: "0.153", min: "0.153.4", want: false},           // short triple is less
	}
	for _, c := range cases {
		if got := codexVersionAtLeast(c.v, c.min); got != c.want {
			t.Errorf("codexVersionAtLeast(%q, %q) = %v, want %v", c.v, c.min, got, c.want)
		}
	}
}

// TestCodexVersionGate_WarnsOnOldBinary: an on-PATH codex older than the
// minimum that ships amazon-bedrock-runtime must produce a visible apply
// warning (stderr) telling the user to update.
func TestCodexVersionGate_WarnsOnOldBinary(t *testing.T) {
	_ = setupApplyTest(t)
	prov := provider.MustGet("codex")
	// Stub PATH + probe (not just the probe): CI runners have no codex binary
	// on PATH, so resolving the real binary would make the gate depend on the
	// host.
	stubCodexProbe(t, "0.148.0-alpha.9")

	stderr := captureStderr(t, func() { warnCodexVersion(prov) })
	for _, want := range []string{"0.148.0-alpha.9", codexMinVersion, provider.CodexBedrockRuntimeProviderID} {
		if !strings.Contains(stderr, want) {
			t.Errorf("warning should name %q, got:\n%s", want, stderr)
		}
	}
}

// TestCodexVersionGate_NoWarnOnModernOrMissingBinary: a modern binary and an
// absent/unparseable binary must NOT warn (absence is the binary-status
// check's job, not the version gate's).
func TestCodexVersionGate_NoWarnOnModernOrMissingBinary(t *testing.T) {
	prov := provider.MustGet("codex")
	t.Run("modern", func(t *testing.T) {
		_ = setupApplyTest(t)
		stubCodexProbe(t, "0.153.4")
		if got := captureStderr(t, func() { warnCodexVersion(prov) }); got != "" {
			t.Errorf("no warning expected, got:\n%s", got)
		}
	})
	t.Run("absent", func(t *testing.T) {
		_ = setupApplyTest(t)
		stubCodexAbsent(t)
		if got := captureStderr(t, func() { warnCodexVersion(prov) }); got != "" {
			t.Errorf("no warning expected, got:\n%s", got)
		}
	})
	t.Run("unparseable", func(t *testing.T) {
		_ = setupApplyTest(t)
		stubCodexProbe(t, "unknown")
		if got := captureStderr(t, func() { warnCodexVersion(prov) }); got != "" {
			t.Errorf("no warning expected, got:\n%s", got)
		}
	})
}

// TestCodexVersionGate_NonCodex: the gate is codex-scoped — other CLIs
// produce no warning even with an old codex on PATH.
func TestCodexVersionGate_NonCodex(t *testing.T) {
	_ = setupApplyTest(t)
	prov := provider.MustGet("grok")
	stubCodexProbe(t, "0.148.0-alpha.9")
	if got := captureStderr(t, func() { warnCodexVersion(prov) }); got != "" {
		t.Errorf("no warning expected, got:\n%s", got)
	}
}

func TestDoctorCodexVersion(t *testing.T) {
	_ = setupApplyTest(t)
	codexProv, err := provider.Get("codex")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		setup   func(t *testing.T)
		want    doctor.Status
		wantSub string
	}{
		{name: "modern", setup: func(t *testing.T) { stubCodexProbe(t, "0.153.4") }, want: doctor.OK, wantSub: "0.153.4"},
		{name: "old", setup: func(t *testing.T) { stubCodexProbe(t, "0.148.0-alpha.9") }, want: doctor.Warn, wantSub: "update"},
		{name: "absent", setup: stubCodexAbsent, want: ""},
		{name: "unparseable", setup: func(t *testing.T) { stubCodexProbe(t, "unknown") }, want: ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.setup(t)

			status, detail := doctorCodexVersion(codexProv)
			if status != c.want {
				t.Fatalf("status = %q, want %q (detail %q)", status, c.want, detail)
			}
			if c.wantSub != "" && !strings.Contains(detail, c.wantSub) {
				t.Errorf("detail %q should contain %q", detail, c.wantSub)
			}
		})
	}
}

// TestDoctorCodexVersion_NonCodex: the check is codex-scoped — other CLIs
// report nothing.
func TestDoctorCodexVersion_NonCodex(t *testing.T) {
	prov, err := provider.Get("grok")
	if err != nil {
		t.Fatal(err)
	}
	if status, _ := doctorCodexVersion(prov); status != "" {
		t.Errorf("non-codex doctor check must be empty, got %q", status)
	}
}
