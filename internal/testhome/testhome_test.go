package testhome

import (
	"os"
	"testing"
)

func TestNewTestHome_SetsHOME(t *testing.T) {
	got := NewTestHome(t)
	if got == "" {
		t.Fatal("expected non-empty path")
	}
	if os.Getenv("HOME") != got {
		t.Errorf("HOME = %q, want %q", os.Getenv("HOME"), got)
	}
}

func TestNewTestHome_SetsUSERPROFILE(t *testing.T) {
	got := NewTestHome(t)
	if os.Getenv("USERPROFILE") != got {
		t.Errorf("USERPROFILE = %q, want %q", os.Getenv("USERPROFILE"), got)
	}
}

func TestNewTestHome_ReturnsTempDir(t *testing.T) {
	got := NewTestHome(t)
	info, err := os.Stat(got)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory")
	}
}
