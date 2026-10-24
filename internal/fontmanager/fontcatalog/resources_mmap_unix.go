//go:build darwin || linux

package fontcatalog

import (
	"errors"
	"os"
	"syscall"
)

func mapFaceFile(file *os.File, size int64) ([]byte, func(), error) {
	if size <= 0 || size > int64(int(^uint(0)>>1)) {
		return nil, nil, errors.New("invalid mapping size")
	}
	mapped, err := syscall.Mmap(int(file.Fd()), 0, int(size), syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		return nil, nil, err
	}
	return mapped, func() { _ = syscall.Munmap(mapped) }, nil
}
