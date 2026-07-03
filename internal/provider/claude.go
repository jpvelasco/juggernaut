package provider

import "runtime"

// claude is the Claude Code provider. Every value here is transcribed verbatim
// from the pre-abstraction hardcoded sites (internal/config/manager.go's
// nativeManagedKeys, internal/activation/activation.go's markers and binary
// names, and the CLAUDE_CODE_USE_BEDROCK launch env var) so behavior is
// byte-identical. The provider_test.go pins guard against drift.
type claude struct{}

func (claude) Name() string { return "claude" }

func (claude) BinaryNames() []string {
	if runtime.GOOS == "windows" {
		return []string{"claude.exe", "claude.cmd", "claude.bat"}
	}
	return []string{"claude"}
}

func (claude) ConfigFormatName() string { return "json" }

func (claude) NativeManagedKeys() []string {
	return []string{
		"env",
		"model",
		"modelOverrides",
		"fallbackModel",
		"effortLevel",
		"alwaysThinkingEnabled",
		"skipWebFetchPreflight",
	}
}

func (claude) ActivationMarkers() (begin, end string) {
	return "# BEGIN: Juggernaut Claude Activation", "# END: Juggernaut Claude Activation"
}

func (claude) BedrockEnvVar() (key, value string) {
	return "CLAUDE_CODE_USE_BEDROCK", "1"
}
