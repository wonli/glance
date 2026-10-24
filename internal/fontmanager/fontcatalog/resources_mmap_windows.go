//go:build windows

package fontcatalog

import (
	"errors"
	"os"
	"syscall"
	"unsafe"
)

func mapFaceFile(file *os.File, size int64) ([]byte, func(), error) {
	if size <= 0 || size > int64(int(^uint(0)>>1)) {
		return nil, nil, errors.New("invalid mapping size")
	}
	low := uint32(size & 0xffffffff)
	high := uint32(size >> 32)
	hMap, err := syscall.CreateFileMapping(syscall.Handle(file.Fd()), nil, syscall.PAGE_READONLY, high, low, nil)
	if err != nil {
		return nil, nil, err
	}
	defer syscall.CloseHandle(hMap)

	addr, err := syscall.MapViewOfFile(hMap, syscall.FILE_MAP_READ, 0, 0, uintptr(size))
	if err != nil {
		return nil, nil, err
	}
	mapped := unsafe.Slice((*byte)(unsafe.Pointer(addr)), size)
	cleanup := func() {
		_ = syscall.UnmapViewOfFile(addr)
	}
	return mapped, cleanup, nil
}
