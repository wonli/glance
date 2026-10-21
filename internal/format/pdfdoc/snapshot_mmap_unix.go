//go:build darwin || linux

package pdfdoc

import (
	"fmt"
	"os"
	"syscall"
)

func mapSnapshotFile(file *os.File, size int64) (*snapshotBacking, error) {
	if size <= 0 || size > int64(int(^uint(0)>>1)) {
		return nil, fmt.Errorf("invalid PDF snapshot mapping size %d", size)
	}
	mapped, err := syscall.Mmap(int(file.Fd()), 0, int(size), syscall.PROT_READ, syscall.MAP_PRIVATE)
	if err != nil {
		return nil, err
	}
	return &snapshotBacking{
		data: mapped,
		release: func() error {
			return syscall.Munmap(mapped)
		},
	}, nil
}
