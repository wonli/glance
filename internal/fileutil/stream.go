package fileutil

import (
	"io"
	"sync"
)

// DefaultBufferSize is the standard chunk size (64KB) used for buffered stream copies.
const DefaultBufferSize = 64 * 1024

var bufferPool = sync.Pool{
	New: func() any {
		b := make([]byte, DefaultBufferSize)
		return &b
	},
}

// GetBuffer acquires a 64KB byte slice from the pool.
// Callers must return it via PutBuffer when done.
func GetBuffer() *[]byte {
	return bufferPool.Get().(*[]byte)
}

// PutBuffer returns a buffer to the pool.
func PutBuffer(buf *[]byte) {
	if buf != nil && len(*buf) == DefaultBufferSize {
		bufferPool.Put(buf)
	}
}

// Copy copies from src to dst using a pooled 64KB buffer to avoid heap allocations.
func Copy(dst io.Writer, src io.Reader) (int64, error) {
	bufPtr := GetBuffer()
	defer PutBuffer(bufPtr)
	return io.CopyBuffer(dst, src, *bufPtr)
}

// CopyWithProgress copies from src to dst using a pooled buffer, invoking onProgress after each chunk.
func CopyWithProgress(dst io.Writer, src io.Reader, onProgress func(written int64)) (int64, error) {
	bufPtr := GetBuffer()
	defer PutBuffer(bufPtr)
	buf := *bufPtr

	var total int64
	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[0:nr])
			if nw < 0 || nr < nw {
				nw = 0
				if ew == nil {
					ew = io.ErrShortWrite
				}
			}
			total += int64(nw)
			if onProgress != nil {
				onProgress(total)
			}
			if ew != nil {
				return total, ew
			}
			if nr != nw {
				return total, io.ErrShortWrite
			}
		}
		if er != nil {
			if er == io.EOF {
				return total, nil
			}
			return total, er
		}
	}
}
