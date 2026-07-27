package provider

import (
	"testing"
)

func TestMustGet_Claude(t *testing.T) {
	p := MustGet("claude")
	if p == nil {
		t.Fatal("MustGet(claude) returned nil")
	}
}

func TestMustGet_PanicsOnUnknown(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("MustGet(nonexistent) did not panic")
		}
	}()
	MustGet("nonexistent-provider")
}
