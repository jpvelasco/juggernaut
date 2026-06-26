//go:build windows

package cmd

import (
	"fmt"
	"os"

	xwindows "github.com/charmbracelet/x/windows"
	"golang.org/x/sys/windows"
)

// flushConsoleInput drains the Windows console input buffer before starting
// a new huh form, preventing stale input events from corrupting the next
// form's input (Bubble Tea v1.x does not do this automatically).
func flushConsoleInput() {
	conin, err := windows.GetStdHandle(windows.STD_INPUT_HANDLE)
	if err != nil {
		return
	}
	if err := xwindows.FlushConsoleInputBuffer(conin); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not flush console input buffer: %v\n", err)
	}
}
