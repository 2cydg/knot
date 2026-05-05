//go:build windows

package fileutil

import "golang.org/x/sys/windows"

func replaceFile(oldpath, newpath string) error {
	oldp, err := windows.UTF16PtrFromString(oldpath)
	if err != nil {
		return err
	}
	newp, err := windows.UTF16PtrFromString(newpath)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(oldp, newp, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
