package provider

import (
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
)

func TestResolveMantleRegion(t *testing.T) {
	cases := []struct {
		name      string
		requested string
		explicit  bool
		known     []string
		want      string
		switched  bool
		message   string
	}{
		{"served region kept", "us-west-2", true, bedrock.MantleRegions, "us-west-2", false, ""},
		{"default region kept", "eu-central-1", false, bedrock.MantleRegions, "eu-central-1", false, ""},
		{
			name:      "unsupported explicit region switches",
			requested: "us-west-1",
			explicit:  true,
			known:     bedrock.MantleRegions,
			want:      "us-east-1",
			switched:  true,
			message:   "requested region us-west-1",
		},
		{
			name:      "unsupported default region switches",
			requested: "eu-west-3",
			explicit:  false,
			known:     bedrock.MantleRegions,
			want:      "us-east-1",
			switched:  true,
			message:   "default region eu-west-3",
		},
		{
			name:      "per-model list narrows the fallback",
			requested: "us-east-1",
			explicit:  true,
			known:     []string{"us-west-2", "us-gov-east-1"},
			want:      "us-west-2",
			switched:  true,
			message:   "using us-west-2 instead",
		},
		{
			name:      "empty known list keeps the requested region",
			requested: "us-east-1",
			explicit:  true,
			known:     nil,
			want:      "us-east-1",
			switched:  false,
			message:   "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, msg, switched := ResolveMantleRegion(tc.requested, tc.explicit, tc.known)
			if got != tc.want {
				t.Errorf("region = %q, want %q", got, tc.want)
			}
			if switched != tc.switched {
				t.Errorf("switched = %v, want %v", switched, tc.switched)
			}
			if tc.message == "" {
				if msg != "" {
					t.Errorf("message = %q, want empty", msg)
				}
				return
			}
			if !strings.Contains(msg, tc.message) {
				t.Errorf("message = %q, want it to contain %q", msg, tc.message)
			}
		})
	}
}

func TestBlockEndpoint(t *testing.T) {
	cases := []struct {
		name  string
		block map[string]any
		want  string
	}{
		{"nil block", nil, ""},
		{"no auth", map[string]any{"meta": map[string]any{}}, ""},
		{"runtime omitted", map[string]any{"auth": map[string]any{"mode": "iam"}}, ""},
		{"mantle", map[string]any{"auth": map[string]any{"endpoint": "mantle"}}, "mantle"},
		{"non-string endpoint", map[string]any{"auth": map[string]any{"endpoint": 1}}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := BlockEndpoint(tc.block); got != tc.want {
				t.Errorf("BlockEndpoint() = %q, want %q", got, tc.want)
			}
		})
	}
}
