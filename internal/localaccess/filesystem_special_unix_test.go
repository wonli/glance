//go:build !windows

package localaccess

import "syscall"

func createSpecialFile(path string) (string, error) {
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		return "", err
	}
	return path, nil
}
