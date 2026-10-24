//go:build darwin || linux

package desktopui

import (
	"errors"
	"os"
	"syscall"
)

func mapFontFile(path string) ([]byte, func(), error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, nil, err
	}
	size := info.Size()
	if size <= 0 || size > int64(int(^uint(0)>>1)) {
		return nil, nil, errors.New("invalid font mapping size")
	}
	mapped, err := syscall.Mmap(int(file.Fd()), 0, int(size), syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		return nil, nil, err
	}
	return mapped, func() { _ = syscall.Munmap(mapped) }, nil
}
