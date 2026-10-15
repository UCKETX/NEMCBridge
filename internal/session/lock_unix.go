//go:build linux || darwin

package session

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func trySessionFileLock(f *os.File) error {
	err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		return ErrBusy
	}
	if err != nil {
		return errors.New("无法锁定机器人会话")
	}
	return nil
}
