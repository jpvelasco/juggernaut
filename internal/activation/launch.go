// launch.go — Launch pipeline, binary resolution, and auth mode detection.

package activation

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
	"github.com/jpvelasco/juggernaut/v5/internal/config"
	"github.com/jpvelasco/juggernaut/v5/internal/keychain"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

// Launch runs Claude Code with Juggernaut Bedrock activation.
func Launch(home string, args []string) error {
	return LaunchCLI(home, args, LaunchTarget{})
}

// LaunchCLI runs the CLI described by target (a zero target defaults to Claude),
// injecting Bedrock env from the keychain. It is the target-aware entry point
// used by the hidden launch commands.
func LaunchCLI(home string, args []string, target LaunchTarget) error {
	return LaunchWithOptions(LaunchOptions{
		Home:        home,
		Args:        args,
		Path:        os.Getenv("PATH"),
		TokenGetter: func() (string, error) { return keychain.Default().GetWithFallback(home) },
		Runner:      RunBinary,
		Target:      target,
	})
}

// LaunchWithOptions runs Claude Code with injectable dependencies.
func LaunchWithOptions(opts LaunchOptions) error {
	if opts.Home == "" {
		return errors.New("home directory is required")
	}
	if opts.Path == "" {
		opts.Path = os.Getenv("PATH")
	}
	if opts.TokenGetter == nil {
		opts.TokenGetter = func() (string, error) { return keychain.Default().GetWithFallback(opts.Home) }
	}
	if opts.Runner == nil {
		opts.Runner = RunBinary
	}
	if opts.Warner == nil {
		opts.Warner = func(msg string) { fmt.Fprintln(os.Stderr, msg) }
	}
	target := opts.Target
	if len(target.BinaryNames) == 0 {
		target = claudeLaunchTarget() // default: Claude (back-compat)
	}
	tokenEnvVar := target.TokenEnvVar
	if tokenEnvVar == "" {
		tokenEnvVar = authmode.BedrockAuthEnvName
	}

	env := os.Environ()
	modes, err := authModes(opts.Home)
	if err != nil {
		return err
	}

	// Determine whether this launch is Juggernaut-managed and whether it needs a
	// bearer token. Claude declares its auth mode in ~/.claude/settings.json
	// (scanned by authModes). Non-Claude CLIs (Codex) store the auth mode in their
	// own config file's juggernaut block — read it from ConfigPath if set.
	// The bearer token is SHARED, so injecting it for a token-needing target
	// is correct regardless of authModes.
	managed := len(modes) > 0 || target.NeedsToken
	wantToken := needsBearerToken(modes) || target.NeedsToken

	// If the target has a config path (non-Claude provider), read the auth mode
	// from its juggernaut block to decide token injection.
	if target.ConfigPath != "" {
		if mode := readAuthModeFromConfig(target.ConfigPath); mode != "" {
			managed = true
			wantToken = authmode.IsBedrockAPIKey(mode)
		}
	}

	if managed {
		for k, v := range target.StaticEnv {
			env = setEnv(env, k, v)
		}
	}
	if wantToken {
		token, err := opts.TokenGetter()
		if err != nil {
			return fmt.Errorf("reading Bedrock API key from keychain: %w", err)
		}
		if token == "" {
			return fmt.Errorf("bedrock API key not found in keychain; run `juggernaut apply --auth=%s`", authmode.BedrockAPIKey)
		}
		// Warn (non-fatally) if a short-term key has already expired — the token
		// is still injected, but Bedrock will reject it, so surface why up front.
		// Juggernaut can't refresh short-term keys (it holds no AWS creds).
		if bedrock.IsAPIKeyExpired(token, time.Now().UTC()) {
			opts.Warner("Warning: your short-term Bedrock API key has expired; regenerate it and run " +
				"`juggernaut apply --auth=" + authmode.BedrockAPIKey + "` (the CLI will fail to authenticate until then)")
		}
		env = setEnv(env, tokenEnvVar, token)
	} else if managed {
		env = unsetEnv(env, tokenEnvVar)
	}

	self, _ := os.Executable()
	binPath, err := resolveBinaryFrom(opts.Path, target.BinaryNames, self, opts.SelfPaths)
	if err != nil {
		return fmt.Errorf("real %s binary not found on PATH", target.BinaryNames[0])
	}
	return opts.Runner(binPath, opts.Args, env)
}

// --- Binary resolution ---

// ResolveClaudeBinary finds the real Anthropic claude command while avoiding
// Juggernaut recursion through old shims or symlinks.
func ResolveClaudeBinary(pathList string) (string, error) {
	self, _ := os.Executable()
	return resolveClaudeBinary(pathList, self)
}

// ResolveBinary resolves the first real executable among names on PATH,
// skipping this process's own executable (wrapper recursion guard).
func ResolveBinary(pathList string, names []string) (string, error) {
	self, _ := os.Executable()
	return resolveBinaryFrom(pathList, names, self, nil)
}

func resolveClaudeBinary(pathList, self string) (string, error) {
	names := []string{"claude"}
	if runtime.GOOS == "windows" {
		names = []string{"claude.exe", "claude.cmd", "claude.bat"}
	}
	return resolveBinaryFrom(pathList, names, self, nil)
}

func resolveBinaryFrom(pathList string, names []string, self string, selfPaths []string) (string, error) {
	for _, dir := range filepath.SplitList(pathList) {
		for _, name := range names {
			candidate := filepath.Join(dir, name)
			if sameExecutable(candidate, self) {
				continue
			}
			// On Windows staged launches, os.Executable() returns the temp copy
			// path. A stale claude.exe/codex.exe hardlinked to the installed
			// binary won't match the temp copy's inode, so also check against
			// any additional self paths passed by the caller.
			skip := false
			for _, sp := range selfPaths {
				if sameExecutable(candidate, sp) {
					skip = true
					break
				}
			}
			if skip {
				continue
			}
			// Reject legacy v4.2.6 claude.cmd/claude.bat shims that invoke
			// the removed juggernaut --launcher path. These shims are not
			// the same file as the juggernaut binary, so sameExecutable
			// alone does not catch them.
			if isKnownJuggernautArtifact(candidate, self) {
				continue
			}
			if isExecutable(candidate) {
				return candidate, nil
			}
		}
	}
	return "", exec.ErrNotFound
}

func sameExecutable(candidate, self string) bool {
	if self == "" {
		return false
	}
	candidateInfo, err := os.Stat(candidate) // #nosec G703 -- candidate is resolved from PATH, not user input
	if err != nil {
		return false
	}
	selfInfo, err := os.Stat(self)
	if err != nil {
		return false
	}
	return os.SameFile(candidateInfo, selfInfo)
}

func isExecutable(path string) bool {
	info, err := os.Stat(path) // #nosec G703 -- path comes from known config paths, not user input
	if err != nil || info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode()&0o111 != 0
}

// --- Auth mode detection ---

func needsBearerToken(modes []string) bool {
	for _, mode := range modes {
		if authmode.IsBedrockAPIKey(mode) {
			return true
		}
	}
	return false
}

// readAuthModeFromConfig reads the auth mode from a provider config file's
// juggernaut block. It handles both JSON and TOML formats based on file
// extension. Returns empty string if the file doesn't exist, has no juggernaut
// block, or can't be parsed.
func readAuthModeFromConfig(path string) string {
	var mgr *config.Manager
	if strings.HasSuffix(path, ".toml") {
		f, err := config.FormatByName("toml")
		if err != nil {
			return ""
		}
		mgr = config.NewManagerWithFormat(path, f)
	} else {
		mgr = config.NewManager(path)
	}
	data, err := mgr.Read()
	if err != nil {
		return ""
	}
	if jb, ok := config.ParseJuggernautBlock(data); ok {
		return jb.AuthMode
	}
	return ""
}

func authModes(home string) ([]string, error) {
	paths := []string{
		filepath.Join(".", ".claude", "settings.json"),
		filepath.Join(home, ".claude", "settings.json"),
	}
	var modes []string
	for _, path := range paths {
		mgr := config.NewManager(path)
		data, err := mgr.Read()
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		if jb, ok := config.ParseJuggernautBlock(data); ok && jb.AuthMode != "" {
			modes = append(modes, jb.AuthMode)
		}
	}
	return modes, nil
}

// --- Environment helpers ---

// RunBinary executes the resolved CLI binary with the given args and environment,
// inheriting stdin/stdout/stderr. Exported for cmd/launch.go which constructs
// LaunchOptions directly instead of using LaunchCLI.
func RunBinary(path string, args []string, env []string) error {
	// path is the resolved real Claude Code executable from ResolveClaudeBinary;
	// exec.Command is intentionally used without a shell to preserve arguments.
	cmd := exec.Command(path, args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command, go_subproc_rule-subproc
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = env
	return cmd.Run()
}

func setEnv(env []string, key, value string) []string {
	prefix := key + "="
	for i, item := range env {
		if strings.HasPrefix(item, prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}

func unsetEnv(env []string, key string) []string {
	prefix := key + "="
	out := env[:0]
	for _, item := range env {
		if !strings.HasPrefix(item, prefix) {
			out = append(out, item)
		}
	}
	return out
}

// --- DefaultBinDir (used by artifact recovery but also by cmd/doctor) ---

// DefaultBinDir returns the user-local bin directory where broken v4.2.6
// artifacts may exist.
func DefaultBinDir(home string) string {
	if home == "" {
		home = safepath.HomeDirOrEmpty()
	}
	path, err := safepath.JoinUnder(home, ".local", "bin")
	if err != nil {
		return filepath.Join(home, ".local", "bin")
	}
	return path
}
