//go:build !windows

package fileutil

import "os"

func ReplaceFile(source, destination string) error {
	return os.Rename(source, destination)
}
