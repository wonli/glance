package desktopinstance

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

const (
	activationProtocolVersion = 1
	activationRetryWindow     = 3 * time.Second
	activationRetryInterval   = 40 * time.Millisecond
	activationFrameLimit      = 256
)

type activationFrame struct {
	Version int    `json:"version"`
	Action  string `json:"action"`
}

// Lease owns the per-user interactive desktop slot and its activation endpoint.
// MCP stdio and headless processes do not acquire a Lease.
type Lease struct {
	lock       interface{ Close() error }
	stopServer func() error
	requests   chan struct{}
	closeOnce  sync.Once
	closeErr   error
}

// AcquireOrActivate returns a lease to the primary desktop process. A secondary
// process returns (nil, false, nil) after the primary acknowledges activation.
func AcquireOrActivate() (*Lease, bool, error) {
	runtimeDir, err := ensureRuntimeDirectory()
	if err != nil {
		return nil, false, err
	}
	deadline := time.Now().Add(activationRetryWindow)
	var lastActivationErr error
	for {
		lock, acquired, err := acquireDesktopLock(runtimeDir)
		if err != nil {
			return nil, false, err
		}
		if acquired {
			requests := make(chan struct{}, 1)
			stopServer, err := startActivationServer(runtimeDir, requests)
			if err != nil {
				_ = lock.Close()
				return nil, false, fmt.Errorf("start desktop activation endpoint: %w", err)
			}
			return &Lease{lock: lock, stopServer: stopServer, requests: requests}, true, nil
		}

		lastActivationErr = sendActivationRequest(runtimeDir)
		if lastActivationErr == nil {
			return nil, false, nil
		}
		if !time.Now().Before(deadline) {
			return nil, false, fmt.Errorf("another Glance desktop process holds the per-user lock but did not accept activation: %w", lastActivationErr)
		}
		time.Sleep(activationRetryInterval)
	}
}

// ActivationRequests is a coalescing stream of path-free raise requests.
func (l *Lease) ActivationRequests() <-chan struct{} {
	if l == nil {
		return nil
	}
	return l.requests
}

// Close stops activation IPC and releases the OS lock. It is safe to call more
// than once, including when the desktop event loop and shutdown worker race.
func (l *Lease) Close() error {
	if l == nil {
		return nil
	}
	l.closeOnce.Do(func() {
		var closeErrors []error
		if l.stopServer != nil {
			if err := l.stopServer(); err != nil {
				closeErrors = append(closeErrors, err)
			}
		}
		if l.lock != nil {
			if err := l.lock.Close(); err != nil {
				closeErrors = append(closeErrors, err)
			}
		}
		l.closeErr = errors.Join(closeErrors...)
	})
	return l.closeErr
}

func readActivationFrame(reader io.Reader) error {
	var size [4]byte
	if _, err := io.ReadFull(reader, size[:]); err != nil {
		return err
	}
	length := binary.BigEndian.Uint32(size[:])
	if length == 0 || length > activationFrameLimit {
		return fmt.Errorf("invalid activation frame length %d", length)
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(reader, data); err != nil {
		return err
	}
	var frame activationFrame
	if err := json.Unmarshal(data, &frame); err != nil {
		return fmt.Errorf("decode activation request: %w", err)
	}
	if frame.Version != activationProtocolVersion || frame.Action != "activate" {
		return errors.New("unsupported desktop activation request")
	}
	return nil
}

func writeActivationFrame(writer io.Writer) error {
	data, err := json.Marshal(activationFrame{Version: activationProtocolVersion, Action: "activate"})
	if err != nil {
		return err
	}
	if len(data) > activationFrameLimit {
		return errors.New("desktop activation request is too large")
	}
	var size [4]byte
	binary.BigEndian.PutUint32(size[:], uint32(len(data)))
	if err := writeActivationBytes(writer, size[:]); err != nil {
		return err
	}
	return writeActivationBytes(writer, data)
}

func writeActivationBytes(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		written, err := writer.Write(data)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		data = data[written:]
	}
	return nil
}
