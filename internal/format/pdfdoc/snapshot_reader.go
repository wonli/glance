package pdfdoc

import (
	"bytes"
	"io"
	"sync"
)

type revisionReader struct {
	mu      sync.Mutex
	reader  *bytes.Reader
	release func() error
	closed  bool
	err     error
}

func (r *revisionReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return 0, io.ErrClosedPipe
	}
	return r.reader.Read(p)
}

func (r *revisionReader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return r.err
	}
	r.closed = true
	if r.release != nil {
		r.err = r.release()
		r.release = nil
	}
	return r.err
}

var _ io.ReadCloser = (*revisionReader)(nil)
