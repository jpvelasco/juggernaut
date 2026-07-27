package testutil

import (
	"os"
	"testing"
)

func TestNestedMapChain_LeafFound(t *testing.T) {
	root := map[string]any{
		"a": map[string]any{
			"b": map[string]any{
				"c": "leaf",
			},
		},
	}
	v, ok := NestedMapChain(root, "a", "b", "c")
	if !ok || v != "leaf" {
		t.Errorf("NestedMapChain = %v, %v want leaf, true", v, ok)
	}
}

func TestNestedMapChain_MissingKey(t *testing.T) {
	root := map[string]any{"a": map[string]any{"b": "x"}}
	_, ok := NestedMapChain(root, "a", "b", "missing")
	if ok {
		t.Error("should not find missing key")
	}
}

func TestNestedMapChain_NonMapIntermediate(t *testing.T) {
	root := map[string]any{"a": map[string]any{"b": "not-a-map"}}
	_, ok := NestedMapChain(root, "a", "b", "c")
	if ok {
		t.Error("should not traverse through a non-map intermediate")
	}
}

func TestNestedMapChain_SingleKey(t *testing.T) {
	root := map[string]any{"top": 42}
	v, ok := NestedMapChain(root, "top")
	if !ok || v != 42 {
		t.Errorf("NestedMapChain = %v, %v want 42, true", v, ok)
	}
}

func TestNestedMapChain_EmptyChain(t *testing.T) {
	root := map[string]any{"a": 1}
	_, ok := NestedMapChain(root)
	if ok {
		t.Error("empty chain should not return found")
	}
}

func TestNestedMapChain_NilRoot(t *testing.T) {
	_, ok := NestedMapChain(nil, "a")
	if ok {
		t.Error("nil root should not return found")
	}
}

func TestNewTestHome(t *testing.T) {
	home := NewTestHome(t)
	if home == "" {
		t.Error("NewTestHome returned empty path")
	}
	if os.Getenv("HOME") != home {
		t.Errorf("HOME env = %q, want %q", os.Getenv("HOME"), home)
	}
	if os.Getenv("USERPROFILE") != home {
		t.Errorf("USERPROFILE env = %q, want %q", os.Getenv("USERPROFILE"), home)
	}
}

func TestCaptureStdout(t *testing.T) {
	got := CaptureStdout(t, func() {
		os.Stdout.WriteString("hello\n")
	})
	if got != "hello\n" {
		t.Errorf("CaptureStdout = %q, want %q", got, "hello\n")
	}
}

func TestWithStdin(t *testing.T) {
	var got string
	WithStdin(t, "answer\n", func() {
		buf := make([]byte, 8)
		n, _ := os.Stdin.Read(buf)
		got = string(buf[:n])
	})
	if got != "answer\n" {
		t.Errorf("stdin = %q, want %q", got, "answer\n")
	}
}
