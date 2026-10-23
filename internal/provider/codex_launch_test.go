package provider

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestCodexLaunchUsesLocalProcess(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"fix the test"},
		{"resume", "--last"},
		{"fork", "--last"},
		{"exec", "--json", "test"},
		{"--model", "app-server", "test"},
		{"-c", `model="agents"`, "resume", "--last"},
		{"--", "--remote"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			original := append([]string(nil), args...)
			got := codexLaunchArgs("codex", args, func(string) ([]byte, error) { return []byte("  --no-daemon  Run without the shared server\n"), nil })
			want := append([]string{"--no-daemon"}, args...)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got %q, want %q", got, want)
			}
			if !reflect.DeepEqual(original, args) {
				t.Fatal("caller arguments mutated")
			}
		})
	}
}

func TestCodexLaunchPreservesExplicitSharedServerAndOldVersions(t *testing.T) {
	for _, args := range [][]string{
		{"--no-daemon"},
		{"--remote", "unix://"},
		{"--remote=unix://"},
		{"resume", "--remote", "unix://", "--last"},
		{"agents"},
		{"app-server", "daemon", "restart"},
		{"remote-control"},
		{"-c", "model='example'", "app-server", "daemon", "start"},
	} {
		got := codexLaunchArgs("codex", args, func(string) ([]byte, error) { t.Fatal("shared/explicit launch should not probe"); return nil, nil })
		if !reflect.DeepEqual(got, args) {
			t.Fatalf("got %q, want %q", got, args)
		}
	}
	for _, help := range []func(string) ([]byte, error){
		func(string) ([]byte, error) { return []byte("older Codex help"), nil },
		func(string) ([]byte, error) { return nil, errors.New("probe failed") },
	} {
		if got := codexLaunchArgs("codex", nil, help); len(got) != 0 {
			t.Fatalf("old/unknown binary received unsupported args: %q", got)
		}
	}
}

func TestCodexPrepareArgsProbeFailurePreservesInvocation(t *testing.T) {
	args := []string{"exec", "prompt"}
	if got := prepareCodexLaunch("nonexistent-codex-test-executable", args); !reflect.DeepEqual(got, args) {
		t.Fatalf("probe failure changed args: %q", got)
	}
	if MustGet("codex").LaunchSpec().PrepareArgs == nil {
		t.Fatal("Codex has no managed launch adapter")
	}
	for _, cli := range []string{"claude", "opencode", "grok"} {
		if MustGet(cli).LaunchSpec().PrepareArgs != nil {
			t.Fatalf("%s unexpectedly modifies launch arguments", cli)
		}
	}
}
