package cmd

import (
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/doctor"
	"github.com/jpvelasco/juggernaut/v5/internal/provider"
)

func TestParseCodexVersion(t *testing.T) {
	cases := []struct {
		out  string
		want string
		ok   bool
	}{
		{out: "codex-cli 0.153.4\n", want: "0.153.4", ok: true},
		{out: "codex-cli 0.148.0-alpha.9", want: "0.148.0-alpha.9", ok: true},
		{out: "0.153.4\n", ok: false}, // no preceding product name
		{out: "\n", ok: false},        // nothing
		{out: "codex-cli\n", ok: false},
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
	restore := codexVersionProbe
	codexVersionProbe = func(string) (string, bool) { return "0.148.0-alpha.9", true }
	defer func() { codexVersionProbe = restore }()

	stderr := captureStderr(t, func() { warnCodexVersion() })
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
	_ = setupApplyTest(t)
	cases := map[string]func(string) (string, bool){
		"modern": func(string) (string, bool) { return "0.153.4", true },
		"absent": func(string) (string, bool) { return "", false },
		"badout": func(string) (string, bool) { return "", false },
	}
	for name, probe := range cases {
		restore := codexVersionProbe
		codexVersionProbe = probe
		defer func() { codexVersionProbe = restore }()
		if got := captureStderr(t, func() { warnCodexVersion() }); got != "" {
			t.Errorf("%s: no warning expected, got:\n%s", name, got)
		}
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
		probe   func(string) (string, bool)
		want    doctor.Status
		wantSub string
	}{
		{name: "modern", probe: func(string) (string, bool) { return "0.153.4", true }, want: doctor.OK, wantSub: "0.153.4"},
		{name: "old", probe: func(string) (string, bool) { return "0.148.0-alpha.9", true }, want: doctor.Warn, wantSub: "update"},
		{name: "absent", probe: func(string) (string, bool) { return "", false }, want: ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			restore := codexVersionProbe
			codexVersionProbe = c.probe
			defer func() { codexVersionProbe = restore }()

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
