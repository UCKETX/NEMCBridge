//go:build windows

package app

import (
	"os"

	"golang.org/x/sys/windows"
)

func enableTerminalColor() bool {
	handle := windows.Handle(os.Stdout.Fd())
	var mode uint32
	if windows.GetConsoleMode(handle, &mode) != nil {
		return false
	}
	return windows.SetConsoleMode(handle, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) == nil
}
