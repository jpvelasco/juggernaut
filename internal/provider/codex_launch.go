package provider

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// prepareCodexLaunch keeps managed AWS credentials in the process that owns
// the session. A shared Codex daemon retains the environment from its start,
// which may predate apply or have a different AWS profile or bearer token.
func prepareCodexLaunch(binary string, args []string) []string {
	return codexLaunchArgs(binary, args, func(path string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return exec.CommandContext(ctx, path, "--help").Output()
	})
}

func codexLaunchArgs(binary string, args []string, help func(string) ([]byte, error)) []string {
	if preserveCodexLaunchArgs(args) {
		return args
	}
	output, err := help(binary)
	if err != nil || !strings.Contains(string(output), "--no-daemon") {
		return args
	}
	return append([]string{"--no-daemon"}, args...)
}

// Only options (not their values) and the first positional command influence
// routing. Respect an explicit remote target or daemon-management command.
func preserveCodexLaunchArgs(args []string) bool {
	commandSeen := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			break
		}
		key, _, hasValue := strings.Cut(arg, "=")
		switch key {
		case "--no-daemon", "--remote":
			return true
		case "-c", "--config", "-m", "--model", "-p", "--profile", "-s", "--sandbox", "-C", "--cd", "--add-dir", "-a", "--ask-for-approval", "--enable", "--disable", "-i", "--image", "--remote-auth-token-env", "--output-schema", "-o", "--output-last-message", "--thread-source":
			if !hasValue {
				i++
			}
		default:
			if strings.HasPrefix(arg, "-") || commandSeen {
				continue
			}
			commandSeen = true
			if arg == "agents" || arg == "app-server" || arg == "remote-control" {
				return true
			}
		}
	}
	return false
}
