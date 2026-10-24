//go:build !windows

package fileutil

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

// ReplaceFile atomically replaces destination with source.
// If source and destination are on different filesystems, it falls back
// to copy-then-replace to support centralized temp directories across devices.
func ReplaceFile(source, destination string) error {
	err := os.Rename(source, destination)
	if err == nil {
		return nil
	}
	var linkErr *os.LinkError
	if errors.As(err, &linkErr) && linkErr.Err == syscall.EXDEV {
		return copyAndReplace(source, destination)
	}
	return err
}

func copyAndReplace(source, destination string) error {
	srcFile, err := os.Open(source)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	destDir := filepath.Dir(destination)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(destDir, ".glance-xdev-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := Copy(tmp, srcFile); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	_ = os.Chmod(tmpName, srcInfo.Mode().Perm())

	if err := os.Rename(tmpName, destination); err != nil {
		return err
	}
	_ = os.Remove(source)
	return nil
}
