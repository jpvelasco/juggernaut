package provider

import "runtime"

// claude is the Claude Code provider. Every value here is transcribed from the
// pre-abstraction sources so behavior is byte-identical: NativeManagedKeys from
// internal/config/manager.go's nativeManagedKeys slice, the markers and binary
// names from internal/activation/activation.go, and the Bedrock env var
// (CLAUDE_CODE_USE_BEDROCK=1) from the literal set inside activation.Launch.
// The provider_test.go pins guard against drift.
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
