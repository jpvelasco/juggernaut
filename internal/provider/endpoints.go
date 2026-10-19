package provider

import (
	"strings"

	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
)

// mantleOnlyReason is the rejection reason shared by every runtime-routed
// provider when a catalog entry exists only on the bedrock-mantle endpoint.
const mantleOnlyReason = "available only on the bedrock-mantle endpoint (opt-in tracked in #497)"

// runtimeUnavailable lists the foundation-model families that AWS documents as
// served only on the bedrock-mantle endpoint. Runtime-routed CLIs must not
// offer them: selecting one writes a config that fails at request time.
//
// Source: https://docs.aws.amazon.com/bedrock/latest/userguide/models-endpoint-availability.html
// Verified 2026-10-19. Keep this list minimal and re-verify it against the
// availability table when adding model families; a model that becomes
// runtime-servable must be removed here.
//
// Matching is segment-aware (`id` or `id-…`) so discovery variants such as
// "google.gemma-4-31b-it" are covered. Claude Mythos 5 is exact on purpose:
// "anthropic.claude-mythos-5-1" is a different model that IS runtime-served.
var runtimeUnavailable = []struct {
	id     string
	family bool
}{
	{id: "anthropic.claude-mythos-5"},
	{id: "google.gemma-4-31b", family: true},
	{id: "google.gemma-4-26b-a4b", family: true},
	{id: "google.gemma-4-e2b", family: true},
	{id: "openai.gpt-5.5", family: true},
	{id: "openai.gpt-5.4", family: true},
	{id: "openai.gpt-5.6-cyber", family: true},
	{id: "openai.gpt-daybreak-blue-5.6-sol", family: true},
	{id: "xai.grok-4.3", family: true},
}

// RuntimeSupported reports whether modelID is callable on bedrock-runtime, the
// endpoint Juggernaut configures for every provider by default.
//
// Inference profiles are runtime-only: mantle has no cross-region inference.
// A foundation model that is not in the verified mantle-only list is assumed
// runtime-supported, so a newly launched runtime model is never hidden by
// omission. Mantle's own allow list is tracked in #497.
func RuntimeSupported(modelID, source string) bool {
	if source == "profile" {
		return true
	}
	if source == "mantle" {
		return false
	}

	bare := bareModelID(modelID)
	for _, entry := range runtimeUnavailable {
		if bare == entry.id {
			return false
		}
		if entry.family && strings.HasPrefix(bare, entry.id+"-") {
			return false
		}
	}
	return true
}

// bareModelID strips a cross-region inference prefix and the version suffix
// used by foundation model IDs ("-v1:0", ":0"). It deliberately does not strip
// model-version segments such as "-5-1": those identify distinct models.
func bareModelID(modelID string) string {
	bare := bedrock.StripRegionPrefix(modelID)
	for _, suffix := range []string{"-v1:0", "-v1", ":0"} {
		bare = strings.TrimSuffix(bare, suffix)
	}
	return bare
}
