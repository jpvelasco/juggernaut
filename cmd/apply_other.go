//go:build !windows

package cmd

// flushConsoleInput is a no-op on non-Windows platforms.
func flushConsoleInput() {
}
