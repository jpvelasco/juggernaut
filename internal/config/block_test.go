package config

import (
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
)

func TestParseJuggernautBlock_Owned(t *testing.T) {
	data := map[string]any{
		"juggernaut": map[string]any{
			"auth": map[string]any{
				"mode":   "iam",
				"region": "us-west-2",
			},
			"meta": map[string]any{
				"managedBy":      "juggernaut",
				"permissionMode": "acceptEdits",
			},
		},
	}
	jb, ok := ParseJuggernautBlock(data)
	if !ok {
		t.Fatal("expected owned block")
	}
	if jb.ManagedBy != "juggernaut" {
		t.Errorf("ManagedBy = %q, want juggernaut", jb.ManagedBy)
	}
	if jb.AuthMode != "iam" {
		t.Errorf("AuthMode = %q, want iam", jb.AuthMode)
	}
	if jb.Region != "us-west-2" {
		t.Errorf("Region = %q, want us-west-2", jb.Region)
	}
	if jb.PermissionMode != "acceptEdits" {
		t.Errorf("PermissionMode = %q, want acceptEdits", jb.PermissionMode)
	}
}

func TestParseJuggernautBlock_NoJuggernautKey(t *testing.T) {
	data := map[string]any{"env": map[string]any{"FOO": "bar"}}
	_, ok := ParseJuggernautBlock(data)
	if ok {
		t.Error("expected no block when juggernaut key is missing")
	}
}

func TestParseJuggernautBlock_WrongManagedBy(t *testing.T) {
	data := map[string]any{
		"juggernaut": map[string]any{
			"meta": map[string]any{
				"managedBy": "someone-else",
			},
		},
	}
	_, ok := ParseJuggernautBlock(data)
	if ok {
		t.Error("expected no block when managedBy != juggernaut")
	}
}

func TestParseJuggernautBlock_NoAuth(t *testing.T) {
	data := map[string]any{
		"juggernaut": map[string]any{
			"meta": map[string]any{
				"managedBy": "juggernaut",
			},
		},
	}
	jb, ok := ParseJuggernautBlock(data)
	if !ok {
		t.Fatal("expected owned block")
	}
	if jb.AuthMode != "" {
		t.Errorf("AuthMode = %q, want empty", jb.AuthMode)
	}
	if jb.Region != "" {
		t.Errorf("Region = %q, want empty", jb.Region)
	}
}

func TestParseJuggernautBlock_EmptyData(t *testing.T) {
	_, ok := ParseJuggernautBlock(map[string]any{})
	if ok {
		t.Error("expected no block for empty data")
	}
}

func TestParseJuggernautBlock_NilData(t *testing.T) {
	_, ok := ParseJuggernautBlock(nil)
	if ok {
		t.Error("expected no block for nil data")
	}
}

func TestParseJuggernautBlock_WithPermissionMode(t *testing.T) {
	data := map[string]any{
		"juggernaut": map[string]any{
			"auth": map[string]any{
				"mode": authmode.BedrockAPIKey,
			},
			"meta": map[string]any{
				"managedBy":      "juggernaut",
				"permissionMode": "auto",
			},
		},
	}
	jb, ok := ParseJuggernautBlock(data)
	if !ok {
		t.Fatal("expected owned block")
	}
	if jb.PermissionMode != "auto" {
		t.Errorf("PermissionMode = %q, want auto", jb.PermissionMode)
	}
}
