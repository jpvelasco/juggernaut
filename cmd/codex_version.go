package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/jpvelasco/juggernaut/v5/internal/activation"
	"github.com/jpvelasco/juggernaut/v5/internal/doctor"
	"github.com/jpvelasco/juggernaut/v5/internal/provider"
)

// codexMinVersion is the minimum Codex CLI that ships the built-in
// amazon-bedrock-runtime provider (which routes to the bedrock-runtime
// endpoint with SigV4). Codex < 0.153.4 lacks that provider, so apply and
// doctor warn the user to update instead of writing a config that would 404 on
// a missing provider. Verified: npm @openai/codex 0.153.4 has it; the
// 0.148.0-alpha.9 native build does not.
const codexMinVersion = "0.153.4"

// codexVersionProbe runs `codex --version` for the given binary path and
// returns the parsed semantic version. A package var so tests substitute a
// stub (the real probe execs the on-PATH codex binary, which is absent in CI).
var codexVersionProbe = func(path string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--version").Output()
	if err != nil {
		return "", false
	}
	return parseCodexVersion(string(out))
}

// parseCodexVersion extracts the semantic version from `codex --version`
// output of the form "codex-cli 0.153.4\n" (the version is the last field).
// A bare version ("0.160.0") is accepted too; callers reject anything that
// doesn't parse into a numeric triple.
func parseCodexVersion(out string) (string, bool) {
	fields := strings.Fields(strings.TrimSpace(out))
	if len(fields) == 0 {
		return "", false
	}
	return fields[len(fields)-1], true
}

// codexVersionPath is the PATH the codex binary is resolved from. A package
// var (defaulted to the real PATH) so tests point it at a temp dir — PATH is
// process-global and CI runners have no codex, so resolving the real binary
// would make the version gate depend on the host.
var codexVersionPath = os.Getenv("PATH")

// codexBinaryVersion resolves the codex binary on PATH and returns its
// version. Returns ok=false when the binary is absent, the probe fails, or the
// version can't be parsed into a numeric triple — callers treat that as
// "cannot determine" and do NOT warn (a missing binary is reported separately
// by the binary-status checks).
func codexBinaryVersion() (string, bool) {
	names := provider.MustGet("codex").BinaryNames()
	found, err := activation.ResolveBinary(codexVersionPath, names)
	if err != nil {
		return "", false
	}
	v, ok := codexVersionProbe(found)
	if !ok || len(versionTriple(v)) == 0 {
		return "", false
	}
	return v, true
}

// codexVersionAtLeast reports whether v is >= min by numeric triple.
func codexVersionAtLeast(v, min string) bool {
	vc := versionTriple(v)
	mc := versionTriple(min)
	for i := 0; i < len(mc); i++ {
		if i >= len(vc) {
			return false
		}
		if vc[i] != mc[i] {
			return vc[i] > mc[i]
		}
	}
	return true
}

// versionTriple splits a semantic version into its numeric components,
// stopping at the first pre-release/build suffix (e.g. "0.148.0-alpha.9" →
// [0 148 0]).
func versionTriple(v string) []int {
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	var out []int
	for _, part := range strings.Split(v, ".") {
		n, err := strconv.Atoi(part)
		if err != nil {
			break
		}
		out = append(out, n)
	}
	return out
}

// warnCodexVersion prints an apply-time warning when the resolved codex binary
// is older than the minimum that ships the built-in amazon-bedrock-runtime
// provider. Self-gates on the codex provider and is a no-op when the binary is
// absent or its version can't be determined.
func warnCodexVersion(prov provider.Provider) {
	if prov.Name() != "codex" {
		return
	}
	v, ok := codexBinaryVersion()
	if !ok || codexVersionAtLeast(v, codexMinVersion) {
		return
	}
	fmt.Fprintf(os.Stderr,
		"⚠ codex %s is older than %s, which is required for the built-in %s provider.\n"+
			"  Juggernaut routes Codex through %s; an older Codex lacks it and requests will 404.\n"+
			"  Update with: npm i -g @openai/codex@latest\n",
		v, codexMinVersion, provider.CodexBedrockRuntimeProviderID, provider.CodexBedrockRuntimeProviderID)
}

// doctorCodexVersion reports the codex CLI version for the doctor report.
// It returns an empty status when the binary is absent or its version can't be
// determined (the separate binary-status check reports that), an OK line when
// the version is adequate, and a Warn line when it is too old.
func doctorCodexVersion(prov provider.Provider) (doctor.Status, string) {
	if prov.Name() != "codex" {
		return "", ""
	}
	v, ok := codexBinaryVersion()
	if !ok {
		return "", ""
	}
	if codexVersionAtLeast(v, codexMinVersion) {
		return doctor.OK, fmt.Sprintf("codex %s (>= %s)", v, codexMinVersion)
	}
	return doctor.Warn, fmt.Sprintf("codex %s is older than %s — update to %s+ for the built-in %s provider (npm i -g @openai/codex@latest)",
		v, codexMinVersion, codexMinVersion, provider.CodexBedrockRuntimeProviderID)
}
