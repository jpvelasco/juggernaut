package activation

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/testutil"
)

func TestLaunchPreparesOnlyTargetManagedArgs(t *testing.T) {
	for _, managed := range []bool{false, true} {
		t.Run(map[bool]string{false: "unmanaged", true: "managed"}[managed], func(t *testing.T) {
			home := testutil.NewTestHome(t)
			dir := t.TempDir()
			name := claudeLaunchTarget().BinaryNames[0]
			binary := filepath.Join(dir, name)
			writeExecutableFile(t, dir, binary, "real executable")
			path := filepath.Join(t.TempDir(), "config.json")
			body := `{}`
			if managed {
				body = `{"juggernaut":{"meta":{"managedBy":"juggernaut"},"auth":{"mode":"iam"}}}`
			}
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			// Another CLI's managed config is not authority to rewrite this
			// target's arguments, even though it activates shared auth handling.
			claudeDir := filepath.Join(home, ".claude")
			if err := os.MkdirAll(claudeDir, 0o700); err != nil { // nosemgrep: go.lang.correctness.permissions.file_permission.incorrect-default-permission -- directory
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(`{"juggernaut":{"meta":{"managedBy":"juggernaut"},"auth":{"mode":"iam"}}}`), 0o600); err != nil {
				t.Fatal(err)
			}
			prepared := false
			exit := errors.New("child exit")
			err := LaunchWithOptions(LaunchOptions{Home: home, Path: dir, Args: []string{"prompt"}, Target: LaunchTarget{
				BinaryNames: []string{name}, ConfigPath: path,
				PrepareArgs: func(got string, args []string) []string {
					prepared = true
					if got != binary {
						t.Fatal("wrong binary probed")
					}
					return append([]string{"--no-daemon"}, args...)
				},
			}, Runner: func(_ string, args []string, _ []string) error {
				want := []string{"prompt"}
				if managed {
					want = []string{"--no-daemon", "prompt"}
				}
				if !reflect.DeepEqual(args, want) {
					t.Fatalf("args=%q want=%q", args, want)
				}
				return exit
			}})
			if !errors.Is(err, exit) || prepared != managed {
				t.Fatalf("err=%v prepared=%v", err, prepared)
			}
		})
	}
}
