//go:build !windows

package main

import "os"

func enableTerminalColor() bool {
	info, err := os.Stdout.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
