package provider

import (
	"runtime"
	"strings"
)

// BaseProvider provides default implementations for common Provider interface
// methods. Embed it in per-CLI provider structs to avoid repeating boilerplate
// for Name, BinaryNames, ConfigFormatName, ActivationMarkers, and Supports.
//
// Each provider only needs to set the struct fields and override the methods
// that are truly unique: ConfigPath, OwnsConfig, NativeManagedKeys,
// DeepMergeKeys, OwnedSubKeys, BuildConfig, and LaunchSpec.
type BaseProvider struct {
	// name is the canonical --cli identifier ("claude", "codex", ...).
	name string
	// displayName is the human-facing CLI name for messages.
	displayName string
	// configFormat reports the on-disk config encoding ("json", "toml").
	configFormat string
	// binaryName is the executable name (without extension) to search on PATH.
	binaryName string
	// capabilities lists all capabilities this provider supports.
	capabilities []Capability
}

// Name returns the canonical provider name.
func (b BaseProvider) Name() string { return b.name }

// BinaryNames returns the platform-specific executable names to search on PATH.
// On Windows it appends .exe, .cmd, and .bat variants; on other platforms it
// returns just the bare name.
func (b BaseProvider) BinaryNames() []string {
	if runtime.GOOS == "windows" {
		return []string{
			b.binaryName + ".exe",
			b.binaryName + ".cmd",
			b.binaryName + ".bat",
		}
	}
	return []string{b.binaryName}
}

// ConfigFormatName returns the config file encoding ("json", "toml").
func (b BaseProvider) ConfigFormatName() string { return b.configFormat }

// ActivationMarkers returns the default begin/end comment markers for shell
// activation blocks. Uses the provider's displayName if set, otherwise
// title-cases the name. Override if your markers differ from the default
// pattern.
func (b BaseProvider) ActivationMarkers() (begin, end string) {
	title := b.displayName
	if title == "" {
		title = strings.Title(b.name) //nolint:staticcheck // ASCII CLI name fallback
	}
	return "# BEGIN: Juggernaut " + title + " Activation", "# END: Juggernaut " + title + " Activation"
}

// Supports reports whether the provider handles a given capability.
func (b BaseProvider) Supports(c Capability) bool {
	for _, cap := range b.capabilities {
		if cap == c {
			return true
		}
	}
	return false
}

// DisplayName returns the human-facing CLI name for messages.
func (b BaseProvider) DisplayName() string {
	if b.displayName != "" {
		return b.displayName
	}
	return strings.Title(b.name) //nolint:staticcheck // ASCII CLI name fallback
}

// SupportedNames returns a sorted, comma-separated list of registered provider names.
// (Exported wrapper around supportedNames for external callers.)
func SupportedNames() string { return supportedNames() }

// Register adds a provider to the global registry.
// (Exported wrapper around register for external callers.)
func Register(p Provider) { register(p) }
