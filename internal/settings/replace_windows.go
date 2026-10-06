//go:build windows

package settings

import "golang.org/x/sys/windows"

// Windows does not expose directory Sync through os.File. Request an atomic
// replacement with write-through instead; errors are returned to the caller.
func replaceDurably(from, to string) (bool, error) {
	src, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return false, err
	}
	dst, err := windows.UTF16PtrFromString(to)
	if err != nil {
		return false, err
	}
	err = windows.MoveFileEx(src, dst, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
	return err == nil, err
}
