//go:build linux || darwin

package push

import (
	"errors"
	"os"
	"syscall"
)

func supportedStorePlatform() error { return nil }
func lockWriter(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}
func unlockWriter(file *os.File) error { return syscall.Flock(int(file.Fd()), syscall.LOCK_UN) }

func openPrivate(path string, flags int) (*os.File, error) {
	fd, err := syscall.Open(path, flags|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if err != nil {
		return nil, errors.New("push state file unavailable")
	}
	file := os.NewFile(uintptr(fd), path)
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		_ = file.Close()
		return nil, errors.New("push state file must be private and regular")
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); !ok || stat.Nlink != 1 {
		_ = file.Close()
		return nil, errors.New("push state file must not have hard links")
	}
	return file, nil
}
