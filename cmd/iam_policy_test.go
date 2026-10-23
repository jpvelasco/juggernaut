package cmd

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
)

func TestIAMPolicyAllowsMantleInference(t *testing.T) {
	data, err := os.ReadFile("../iam-policy.json")
	if err != nil {
		t.Fatal(err)
	}
	var policy struct {
		Statement []struct {
			Effect   string
			Action   []string
			Resource any
		}
	}
	if err := json.Unmarshal(data, &policy); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"bedrock-mantle:CreateInference", "bedrock-mantle:CountTokens", "bedrock-mantle:ListModels"} {
		found := false
		for _, s := range policy.Statement {
			if s.Effect == "Allow" && slices.Contains(s.Action, action) {
				want := "arn:aws:bedrock-mantle:*:*:project/*"
				if action == "bedrock-mantle:ListModels" {
					want = "*"
				}
				if s.Resource != want {
					t.Fatalf("%s has incorrect resource %v", action, s.Resource)
				}
				found = true
			}
		}
		if !found {
			t.Errorf("policy lacks %s", action)
		}
	}
}
