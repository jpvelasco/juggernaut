//go:build windows

package cmd

import "testing"

func TestFlushConsoleInput_NoPanic(t *testing.T) {
	flushConsoleInput()
}
