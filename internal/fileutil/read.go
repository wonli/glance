package fileutil

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ErrNotRegularFile is returned when a path does not point to a regular file.
var ErrNotRegularFile = errors.New("not a regular file")

// ErrFileTooLarge is returned when a file exceeds the allowed read limit.
var ErrFileTooLarge = errors.New("file exceeds maximum allowed size")

// OpenStream safely opens path for reading, verifying it is a regular file and returning its size.
// The caller is responsible for closing the returned *os.File.
func OpenStream(path string) (*os.File, int64, error) {
	cleanPath := filepath.Clean(path)
	file, err := os.Open(cleanPath)
	if err != nil {
		return nil, 0, err
	}

	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, 0, err
	}

	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, 0, fmt.Errorf("%w: %q is a directory or special device", ErrNotRegularFile, cleanPath)
	}

	return file, info.Size(), nil
}

// OpenBuffered opens path as a buffered reader with the given buffer size (or DefaultBufferSize if <= 0).
// The caller must call Close() on the returned io.Closer.
func OpenBuffered(path string, bufSize int) (*bufio.Reader, io.Closer, int64, error) {
	file, size, err := OpenStream(path)
	if err != nil {
		return nil, nil, 0, err
	}
	if bufSize <= 0 {
		bufSize = DefaultBufferSize
	}
	return bufio.NewReaderSize(file, bufSize), file, size, nil
}

// ReadLimit reads up to maxBytes from path. If the file is larger than maxBytes,
// it returns ErrFileTooLarge without loading the whole file into memory.
func ReadLimit(path string, maxBytes int64) ([]byte, error) {
	file, size, err := OpenStream(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	if maxBytes > 0 && size > maxBytes {
		return nil, fmt.Errorf("%w: size %d exceeds limit %d", ErrFileTooLarge, size, maxBytes)
	}

	capacity := size
	if maxBytes > 0 && capacity > maxBytes {
		capacity = maxBytes
	}
	buf := make([]byte, 0, capacity)
	limited := io.LimitReader(file, maxBytes+1)
	for {
		chunk := GetBuffer()
		n, rErr := limited.Read(*chunk)
		if n > 0 {
			if maxBytes > 0 && int64(len(buf)+n) > maxBytes {
				PutBuffer(chunk)
				return nil, fmt.Errorf("%w: stream exceeded limit %d", ErrFileTooLarge, maxBytes)
			}
			buf = append(buf, (*chunk)[:n]...)
		}
		PutBuffer(chunk)
		if rErr != nil {
			if rErr == io.EOF {
				break
			}
			return nil, rErr
		}
	}
	return buf, nil
}

// ReadExact reads the entire contents of a regular file.
// It preallocates the buffer matching FileInfo.Size to eliminate dynamic slice reallocations.
func ReadExact(path string) ([]byte, error) {
	file, size, err := OpenStream(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	if size == 0 {
		return []byte{}, nil
	}

	data := make([]byte, size)
	_, err = io.ReadFull(file, data)
	if err != nil {
		return nil, err
	}
	return data, nil
}
