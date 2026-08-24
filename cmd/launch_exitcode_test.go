package cmd

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("JUGGERNAUT_TEST_WRAPPER_CHILD") == "1" {
		wrapperChildMain()
		return
	}
	os.Exit(m.Run())
}

// wrapperChildMain runs inside a freshly spawned test-binary process and acts
// as the wrapper CLI (`juggernaut launch-cli claude -- --version`).
func wrapperChildMain() {
	rootCmd.SetArgs([]string{"launch-cli", "claude", "--", "--version"})
	Execute()
}

// writeClaudeStub plants a fake `claude` executable that exits with code at
// the front of PATH, so the launch pipeline resolves and runs it.
func writeClaudeStub(t *testing.T, dir string, code int) {
	t.Helper()
	var stub string
	var body string
	if runtime.GOOS == "windows" {
		stub = filepath.Join(dir, "claude.cmd")
		body = "@echo off\r\nexit /b " + strconv.Itoa(code) + "\r\n"
	} else {
		stub = filepath.Join(dir, "claude")
		body = "#!/bin/sh\nexit " + strconv.Itoa(code) + "\n"
	}
	if err := os.WriteFile(stub, []byte(body), 0o755); err != nil { // #nosec G306 -- stub must be executable
		t.Fatalf("writing claude stub: %v", err)
	}
}

// Given Juggernaut wraps a CLI whose process exits with a non-zero code,
// When a script runs the wrapped CLI through `launch-cli` and checks $?,
// Then the wrapper exits with the child's exact code and prints neither an
// error line, nor a usage dump, nor internal "exit status N" noise.
func TestExecute_PropagatesLaunchedCLIExitCode(t *testing.T) {
	home := t.TempDir()
	stubDir := filepath.Join(home, "bin")
	if err := os.MkdirAll(stubDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeClaudeStub(t, stubDir, 2)

	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("JUGGERNAUT_TEST_WRAPPER_CHILD", "1")

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("locating test binary: %v", err)
	}
	out, err := exec.Command(exe).CombinedOutput() // #nosec G204 -- spawns this test binary in wrapper-child mode
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("expected ExitError from wrapper child, got %v; output:\n%s", err, out)
	}
	if ee.ExitCode() != 2 {
		t.Errorf("wrapper exit code = %d, want child's code 2; output:\n%s", ee.ExitCode(), out)
	}
	for _, noise := range []string{"exit status", "Error:", "Usage:"} {
		if strings.Contains(string(out), noise) {
			t.Errorf("wrapper leaked %q to output:\n%s", noise, out)
		}
	}
}
