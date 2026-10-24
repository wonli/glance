package pdfdoc

import (
	"errors"
	"os"
	"sync"
)

// snapshotBacking owns the bytes referenced by gopdf.Reader and releases the
// underlying mapping or compatibility allocation when its Document closes.
type snapshotBacking struct {
	data     []byte
	release  func() error
	tempPath string
	mu       sync.Mutex
	leases   int
	closing  bool
	closed   bool
	closeErr error
}

func (b *snapshotBacking) Close() error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || b.closing {
		return b.closeErr
	}
	b.closing = true
	if b.leases > 0 {
		return nil
	}
	return b.closeLocked()
}

func (b *snapshotBacking) acquire() (func() error, error) {
	if b == nil {
		return nil, errors.New("PDF snapshot backing is unavailable")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closing || b.closed || b.release == nil {
		return nil, errors.New("PDF snapshot backing is closing")
	}
	b.leases++
	var once sync.Once
	var releaseErr error
	return func() error {
		once.Do(func() { releaseErr = b.releaseLease() })
		return releaseErr
	}, nil
}

func (b *snapshotBacking) releaseLease() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.leases > 0 {
		b.leases--
	}
	if b.closing && b.leases == 0 && !b.closed {
		return b.closeLocked()
	}
	return nil
}

func (b *snapshotBacking) closeLocked() error {
	if b.closed {
		return b.closeErr
	}
	b.closed = true
	if b.release != nil {
		b.closeErr = b.release()
	}
	b.release = nil
	b.data = nil
	if b.tempPath != "" {
		if err := os.Remove(b.tempPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			b.closeErr = errors.Join(b.closeErr, err)
		}
		b.tempPath = ""
	}
	return b.closeErr
}
