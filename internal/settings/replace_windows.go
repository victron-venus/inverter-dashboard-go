//go:build windows

package settings

import "golang.org/x/sys/windows"

// Windows does not expose directory Sync through os.File. Request an atomic
// replacement with write-through instead; errors are returned to the caller.
func replaceDurably(from, to string) error {
	src, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	dst, err := windows.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(src, dst, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
