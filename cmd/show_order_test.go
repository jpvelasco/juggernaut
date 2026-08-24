package cmd

import (
	"io"
	"os"
	"strings"
	"testing"
)

// captureStreaming runs fn while streaming os.Stdout into a buffer from a
// concurrent reader goroutine, so large outputs cannot deadlock on the OS
// pipe capacity (unlike the drain-after-capture helper).
func captureStreaming(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	defer func() { os.Stdout = orig }()
	fn()
	os.Stdout = orig
	if err := w.Close(); err != nil {
		t.Fatalf("closing stdout pipe: %v", err)
	}
	out := <-done
	_ = r.Close()
	return out
}

// Given an identical on-disk configuration,
// When `juggernaut show` runs repeatedly without --json,
// Then the text output must be byte-identical every run — scripts diff and
// log this output, so map-iteration randomness is user-visible churn.
func TestShow_TextOutputDeterministic(t *testing.T) {
	setupApplyTestWithReset(t)
	if err := ExecuteArgs([]string{"apply", "--auth=iam", "--region=us-west-2", "--skip-preflight"}); err != nil {
		t.Fatalf("apply: %v", err)
	}

	seen := map[string]bool{}
	for i := 0; i < 40; i++ {
		out := captureStreaming(t, func() {
			if err := ExecuteArgs([]string{"show"}); err != nil {
				t.Fatalf("show: %v", err)
			}
		})
		seen[out] = true
	}
	if len(seen) != 1 {
		t.Fatalf("show text output is nondeterministic: %d distinct outputs across 40 identical-state runs", len(seen))
	}
}

// Given both scopes are configured,
// When `juggernaut show` prints its text sections,
// Then they appear in the canonical resolvedScopes order (user, project).
func TestShow_TextOutputSectionOrder(t *testing.T) {
	setupApplyTestWithReset(t)
	if err := ExecuteArgs([]string{"apply", "--auth=iam", "--region=us-west-2", "--skip-preflight"}); err != nil {
		t.Fatalf("apply: %v", err)
	}

	configBytes, err := os.ReadFile(findBedrockConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	originalEmbedded := embeddedConfigBytes
	SetEmbeddedConfig(configBytes)
	t.Cleanup(func() { SetEmbeddedConfig(originalEmbedded) })
	t.Chdir(t.TempDir())
	if err := ExecuteArgs([]string{
		"apply", "--scope=project", "--auth=iam", "--region=us-west-2", "--skip-preflight",
	}); err != nil {
		t.Fatalf("project apply: %v", err)
	}

	out := captureStreaming(t, func() {
		if err := ExecuteArgs([]string{"show"}); err != nil {
			t.Fatalf("show: %v", err)
		}
	})

	userIdx := strings.Index(out, "=== user scope ===")
	projectIdx := strings.Index(out, "=== project scope ===")
	if userIdx < 0 || projectIdx < 0 {
		t.Fatalf("missing scope sections in output:\n%s", out)
	}
	if userIdx > projectIdx {
		t.Errorf("sections printed out of canonical order (user should precede project):\n%s", out)
	}
}
