package cmd

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/activation"
	"github.com/jpvelasco/juggernaut/v5/internal/provider"
)

// A migrated user sidecar must not shadow a still-legacy project's auth state.
func TestSidecarLaunchPreservesLegacyProjectPrecedence(t *testing.T) {
	for _, cli := range []string{"codex", "opencode"} {
		for _, mode := range []string{"iam", "bedrock-api-key"} {
			t.Run(cli+"/"+mode, func(t *testing.T) {
				home := setupApplyTestWithReset(t)
				mantleProjectDir(t)
				t.Setenv("AWS_PROFILE", "")
				p := provider.MustGet(cli)
				userMode := "iam"
				if mode == "iam" {
					userMode = "bedrock-api-key"
				}
				if err := provider.WriteSidecar(p, home, provider.Options{Scope: "user", AuthMode: userMode, Profile: "user"}); err != nil {
					t.Fatal(err)
				}
				mgr, err := newProviderManager(p, home, "project")
				if err != nil {
					t.Fatal(err)
				}
				if err := mgr.Write(map[string]any{"juggernaut": map[string]any{
					"meta": map[string]any{"managedBy": "juggernaut"},
					"auth": map[string]any{"mode": mode, "profile": "project"},
				}}); err != nil {
					t.Fatal(err)
				}
				dir := t.TempDir()
				writeExecutableStub(t, filepath.Join(dir, p.BinaryNames()[0]))
				target := launchTargetFor(p, "")
				target.ConfigPaths = resolveLaunchConfigPaths(p, home)
				gotToken := false
				err = activation.LaunchWithOptions(activation.LaunchOptions{
					Home: home, Path: dir, Target: target,
					TokenGetter: func() (string, error) { gotToken = true; return "test-key", nil },
					Runner: func(_ string, _ []string, env []string) error {
						if !slices.Contains(env, "AWS_PROFILE=project") {
							t.Fatal("user profile shadowed legacy project")
						}
						return nil
					},
				})
				if err != nil {
					t.Fatal(err)
				}
				if gotToken != (mode == "bedrock-api-key") {
					t.Fatal("user sidecar shadowed project auth mode")
				}
				doctorMode, doctorProfile := managedAuthModeAndProfile(p, home, []string{"user", "project"})
				if doctorMode != mode || doctorProfile != "project" {
					t.Fatalf("doctor disagrees with launch: %s %s", doctorMode, doctorProfile)
				}
				doctorMode, doctorProfile = managedAuthModeAndProfile(p, home, []string{"user"})
				if doctorMode != userMode || doctorProfile != "user" {
					t.Fatalf("doctor ignored scope filter: %s %s", doctorMode, doctorProfile)
				}
			})
		}
	}
}
