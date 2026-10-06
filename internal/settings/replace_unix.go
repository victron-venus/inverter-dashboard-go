//go:build !windows

package settings

import (
	"os"
	"path/filepath"
)

func replaceDurably(from, to string) (bool, error) {
	if err := os.Rename(from, to); err != nil {
		return false, err
	}
	dir, err := os.Open(filepath.Dir(to))
	if err != nil {
		return true, err
	}
	defer func() { _ = dir.Close() }()
	return true, dir.Sync()
}
