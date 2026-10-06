//go:build !linux && !darwin

package push

import (
	"errors"
	"os"
)

func supportedStorePlatform() error {
	return errors.New("persistent Web Push is supported only on Linux and macOS")
}
func openPrivate(_ string, _ int) (*os.File, error) { return nil, supportedStorePlatform() }
func lockWriter(_ *os.File) error                   { return supportedStorePlatform() }
func unlockWriter(_ *os.File) error                 { return nil }
