package fileutil

import (
	"errors"
	"os"
	"path/filepath"
)

// MoveExclusive atomically or safely places source at destination,
// failing immediately if destination already exists.
// On success, source is removed.
func MoveExclusive(source, destination string) error {
	destDir := filepath.Dir(destination)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	err := os.Link(source, destination)
	if err == nil {
		_ = os.Remove(source)
		return nil
	}
	if errors.Is(err, os.ErrExist) {
		return err
	}

	// If link fails due to cross-device (EXDEV) or unsupported hardlink,
	// fall back to exclusive file creation with streamed copy.
	return copyExclusive(source, destination)
}

func copyExclusive(source, destination string) error {
	srcFile, err := os.Open(source)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	dstFile, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, srcInfo.Mode().Perm())
	if err != nil {
		return err
	}
	var committed bool
	defer func() {
		if !committed {
			_ = dstFile.Close()
			_ = os.Remove(destination)
		}
	}()

	if _, err := Copy(dstFile, srcFile); err != nil {
		_ = dstFile.Close()
		return err
	}
	if err := dstFile.Sync(); err != nil {
		_ = dstFile.Close()
		return err
	}
	if err := dstFile.Close(); err != nil {
		return err
	}
	committed = true
	_ = os.Remove(source)
	return nil
}
