package bedrock

import (
	"slices"
	"strings"
)

// Endpoint identifiers for the two Bedrock inference endpoints.
const (
	// EndpointRuntime is the default InvokeModel / Converse endpoint every
	// Juggernaut provider uses.
	EndpointRuntime = "runtime"
	// EndpointMantle is the opt-in OpenAI/Anthropic-shaped endpoint.
	EndpointMantle = "mantle"
)

// ValidEndpoint reports whether endpoint is a supported endpoint identifier.
// The empty string is not accepted here; callers normalize it to runtime.
func ValidEndpoint(endpoint string) bool {
	return endpoint == EndpointRuntime || endpoint == EndpointMantle
}

// MantleRegions lists the AWS regions that serve the bedrock-mantle endpoint,
// in AWS documentation order. The first entry is the Iron Fist fallback for a
// requested region that does not serve mantle.
//
// Source: https://docs.aws.amazon.com/bedrock/latest/userguide/endpoints-region-availability.html
// Verified 2026-10-19. Mantle is a subset of runtime; keep this list ordered
// and re-verify it when AWS adds or removes a region.
var MantleRegions = []string{
	"us-east-1", "us-east-2", "us-west-2",
	"eu-central-1", "eu-north-1", "eu-south-1", "eu-west-1", "eu-west-2",
	"ap-northeast-1", "ap-south-1", "ap-southeast-2", "ap-southeast-3",
	"sa-east-1",
	"us-gov-east-1", "us-gov-west-1",
}

// MantleRegionSupported reports whether region serves bedrock-mantle.
func MantleRegionSupported(region string) bool {
	return slices.Contains(MantleRegions, region)
}

// IsMantleModelID reports whether modelID uses the Mantle model-ID format: an
// anthropic.* foundation-model ID with no cross-Region inference prefix and no
// version suffix. Mantle serves the Anthropic Messages API only for Claude
// models and does not support cross-Region inference; Claude Code routes model
// IDs by this format when both endpoints are enabled.
//
// The optional Claude Code [1m] context annotation is ignored because it is a
// client suffix, not part of the model identifier. The format check is
// deliberately not an allowlist: each account's Mantle lineup is granted
// separately, so Juggernaut cannot know which IDs are callable.
func IsMantleModelID(modelID string) bool {
	id := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(modelID), "[1m]"))
	if !strings.HasPrefix(id, "anthropic.") || len(id) == len("anthropic.") {
		return false
	}
	if StripRegionPrefix(id) != id {
		return false
	}
	for _, suffix := range []string{"-v1:0", "-v1", ":0"} {
		if strings.HasSuffix(id, suffix) {
			return false
		}
	}
	return true
}
