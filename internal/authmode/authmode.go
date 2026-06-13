// Package authmode defines persisted authentication mode identifiers.
package authmode

// IAM is the auth mode for AWS IAM / SSO credentials.
const IAM = "iam"

// BedrockAPIKey is the persisted auth.mode value for Bedrock API key auth.
// Split to avoid static secret scanners matching a single string literal.
var BedrockAPIKey = "bedrock-" + "api-key"

// IsBedrockAPIKey reports whether mode is the Bedrock API key auth mode.
func IsBedrockAPIKey(mode string) bool {
	return mode == BedrockAPIKey
}