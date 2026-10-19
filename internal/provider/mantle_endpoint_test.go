package provider

import (
	"strings"
	"testing"
)

func TestResolveEndpointRegion(t *testing.T) {
	cases := []struct {
		name      string
		requested string
		explicit  bool
		want      string
		switched  bool
		message   string
	}{
		{"served region kept", "us-west-2", true, "us-west-2", false, ""},
		{"default region kept", "eu-central-1", false, "eu-central-1", false, ""},
		{
			name:      "unsupported explicit region switches",
			requested: "us-west-1",
			explicit:  true,
			want:      "us-east-1",
			switched:  true,
			message:   "requested region us-west-1",
		},
		{
			name:      "unsupported default region switches",
			requested: "eu-west-3",
			explicit:  false,
			want:      "us-east-1",
			switched:  true,
			message:   "default region eu-west-3",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, msg, switched := ResolveEndpointRegion(tc.requested, tc.explicit)
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
