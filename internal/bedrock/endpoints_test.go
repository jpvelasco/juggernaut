package bedrock

import "testing"

func TestValidEndpoint(t *testing.T) {
	for _, endpoint := range []string{EndpointRuntime, EndpointMantle} {
		if !ValidEndpoint(endpoint) {
			t.Errorf("ValidEndpoint(%q) = false, want true", endpoint)
		}
	}
	for _, endpoint := range []string{"", "bedrock-runtime", "MANTLE", " mantle"} {
		if ValidEndpoint(endpoint) {
			t.Errorf("ValidEndpoint(%q) = true, want false", endpoint)
		}
	}
}

func TestMantleRegionSupported(t *testing.T) {
	for _, region := range []string{"us-east-1", "us-west-2", "eu-central-1", "ap-southeast-3", "us-gov-west-1"} {
		if !MantleRegionSupported(region) {
			t.Errorf("MantleRegionSupported(%q) = false, want true", region)
		}
	}
	for _, region := range []string{"", "us-west-1", "eu-west-3", "ap-northeast-2", "US-EAST-1"} {
		if MantleRegionSupported(region) {
			t.Errorf("MantleRegionSupported(%q) = true, want false", region)
		}
	}
}

func TestIsMantleModelID(t *testing.T) {
	cases := []struct {
		name string
		id   string
		want bool
	}{
		{"bare sonnet", "anthropic.claude-sonnet-5", true},
		{"bare haiku", "anthropic.claude-haiku-4-5", true},
		{"bare opus", "anthropic.claude-opus-4-7", true},
		{"context annotation", "anthropic.claude-sonnet-5[1m]", true},
		{"surrounding whitespace", "  anthropic.claude-sonnet-5  ", true},

		{"global CRIS prefix", "global.anthropic.claude-sonnet-5", false},
		{"US CRIS prefix", "us.anthropic.claude-sonnet-5", false},
		{"GovCloud CRIS prefix", "us-gov.anthropic.claude-sonnet-5", false},
		{"EU CRIS prefix", "eu.anthropic.claude-sonnet-5", false},
		{"APAC CRIS prefix", "apac.anthropic.claude-sonnet-5", false},
		{"version suffix", "anthropic.claude-sonnet-5-v1:0", false},
		{"version suffix no revision", "anthropic.claude-sonnet-5-v1", false},
		{"empty version suffix", "anthropic.claude-sonnet-5:0", false},
		{"runtime foundation ID", "anthropic.claude-haiku-4-5-20251001-v1:0", false},
		{"non-anthropic model", "openai.gpt-6.1-sol", false},
		{"ARN", "arn:aws:bedrock:us-west-2::foundation-model/anthropic.claude-sonnet-5", false},
		{"provider only", "anthropic.", false},
		{"empty", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsMantleModelID(tc.id); got != tc.want {
				t.Errorf("IsMantleModelID(%q) = %v, want %v", tc.id, got, tc.want)
			}
		})
	}
}
