package cmd

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/activation"
	"github.com/jpvelasco/juggernaut/v5/internal/provider"
)

func TestMantleLaunchNativeAuthAndSharedKey(t *testing.T) {
	for _, cli := range []string{"codex", "opencode"} {
		for _, mode := range []string{"iam", "bedrock-api-key"} {
			t.Run(cli+"/"+mode, func(t *testing.T) {
				home := setupApplyTestWithReset(t)
				t.Setenv("AWS_PROFILE", "")
				t.Setenv("AWS_BEARER_TOKEN_BEDROCK", "stale-parent-key")
				p := provider.MustGet(cli)
				opts := provider.Options{Scope: "user", Endpoint: "mantle", Region: "us-west-2", AuthMode: mode, Profile: "work"}
				plan, err := p.BuildConfig(nil, opts)
				if err != nil {
					t.Fatal(err)
				}
				mgr, err := newProviderManager(p, home, "user")
				if err != nil {
					t.Fatal(err)
				}
				if err := mgr.Write(plan.Keys); err != nil {
					t.Fatal(err)
				}
				if err := provider.WriteSidecar(p, home, opts); err != nil {
					t.Fatal(err)
				}
				dir := t.TempDir()
				writeExecutableStub(t, filepath.Join(dir, p.BinaryNames()[0]))
				target := launchTargetFor(p, "")
				target.ConfigPaths = resolveLaunchConfigPaths(p, home)
				called := false
				exit := errors.New("child status")
				err = activation.LaunchWithOptions(activation.LaunchOptions{
					Home: home, Path: dir, Target: target,
					TokenGetter: func() (string, error) { called = true; return "shared-test-key", nil },
					Runner: func(_ string, _ []string, env []string) error {
						values := map[string]string{}
						for _, pair := range env {
							k, v, _ := strings.Cut(pair, "=")
							values[k] = v
						}
						want := ""
						if mode == "bedrock-api-key" {
							want = "shared-test-key"
						}
						if values["AWS_BEARER_TOKEN_BEDROCK"] != want {
							t.Fatal("incorrect bearer injection")
						}
						if mode == "iam" && values["AWS_PROFILE"] != "work" {
							t.Fatal("saved IAM profile not exported")
						}
						return exit
					},
				})
				if !errors.Is(err, exit) || called != (mode == "bedrock-api-key") {
					t.Fatalf("launch: %v, keychain called=%v", err, called)
				}
			})
		}
	}
}
