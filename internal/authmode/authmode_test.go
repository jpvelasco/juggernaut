package authmode

import "testing"

func TestIsBedrockAPIKey(t *testing.T) {
	tests := []struct {
		name string
		mode string
		want bool
	}{
		{name: "bedrock api key mode", mode: BedrockAPIKey, want: true},
		{name: "iam mode", mode: IAM, want: false},
		{name: "empty mode", mode: "", want: false},
		{name: "unknown mode", mode: "something-else", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsBedrockAPIKey(tt.mode); got != tt.want {
				t.Errorf("IsBedrockAPIKey(%q) = %v, want %v", tt.mode, got, tt.want)
			}
		})
	}
}

func TestBedrockAPIKeyValue(t *testing.T) {
	if BedrockAPIKey != "bedrock-api-key" {
		t.Errorf("BedrockAPIKey = %q, want %q", BedrockAPIKey, "bedrock-api-key")
	}
	if IAM != "iam" {
		t.Errorf("IAM = %q, want %q", IAM, "iam")
	}
}
