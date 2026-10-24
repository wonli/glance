package fileutil

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

var (
	customTempDirMu sync.RWMutex
	customTempDir   string
)

// SetDefaultTempDir sets a custom directory for Glance temporary files.
func SetDefaultTempDir(dir string) {
	customTempDirMu.Lock()
	defer customTempDirMu.Unlock()
	customTempDir = dir
}

// DefaultTempDir returns the centralized temporary directory for Glance.
// All temporary saving files are placed here rather than polluting user document folders.
func DefaultTempDir() string {
	customTempDirMu.RLock()
	dir := customTempDir
	customTempDirMu.RUnlock()
	if dir != "" {
		_ = os.MkdirAll(dir, 0700)
		return dir
	}

	if configDir, err := os.UserConfigDir(); err == nil && configDir != "" {
		glanceTmp := filepath.Join(configDir, "Glance Office", "tmp")
		if err := os.MkdirAll(glanceTmp, 0700); err == nil {
			return glanceTmp
		}
	}

	fallback := filepath.Join(os.TempDir(), "glance-tmp")
	_ = os.MkdirAll(fallback, 0700)
	return fallback
}

// CreateTemp creates a temporary file in Glance's dedicated temporary directory.
func CreateTemp(pattern string) (*os.File, error) {
	dir := DefaultTempDir()
	return os.CreateTemp(dir, pattern)
}

// WriteOptions configures atomic write operations.
type WriteOptions struct {
	Perm               os.FileMode
	CommitGuard        func() (release func(), err error)
	FileCommitGuard    func(tmpPath string) (release func(), err error)
	TempPattern        string
	TempDir            string
	AutoCreateDir      bool
	PreserveTargetPerm bool
}

// WriteOption modifies WriteOptions.
type WriteOption func(*WriteOptions)

// WithPerm specifies the file permission mode for newly created files (e.g. 0644).
func WithPerm(mode os.FileMode) WriteOption {
	return func(o *WriteOptions) {
		o.Perm = mode
	}
}

// WithCommitGuard supplies a verification hook that runs after the temporary file
// is fully written, flushed, and synced, but immediately before replacing the target file.
// If guard returns an error, the write operation is aborted, the temporary file is removed,
// and the original target file remains untouched.
// If the guard returns a release closure, it is invoked after the replacement attempt completes.
func WithCommitGuard(guard func() (func(), error)) WriteOption {
	return func(o *WriteOptions) {
		o.CommitGuard = guard
	}
}

// WithFileCommitGuard supplies a verification hook that receives the temporary file path
// to validate file structure on disk before replacing the target file.
func WithFileCommitGuard(guard func(tmpPath string) (func(), error)) WriteOption {
	return func(o *WriteOptions) {
		o.FileCommitGuard = guard
	}
}

// WithTempPattern customizes the temporary filename pattern (e.g. ".glance-*.docx").
func WithTempPattern(pattern string) WriteOption {
	return func(o *WriteOptions) {
		o.TempPattern = pattern
	}
}

// WithTempDir specifies a custom directory for temporary files.
func WithTempDir(dir string) WriteOption {
	return func(o *WriteOptions) {
		o.TempDir = dir
	}
}

// WithAutoCreateDir specifies whether parent directories should be automatically created.
func WithAutoCreateDir(create bool) WriteOption {
	return func(o *WriteOptions) {
		o.AutoCreateDir = create
	}
}

// WithPreserveTargetPerm specifies that if the target file already exists,
// its existing file permissions should be preserved.
func WithPreserveTargetPerm(preserve bool) WriteOption {
	return func(o *WriteOptions) {
		o.PreserveTargetPerm = preserve
	}
}

func defaultWriteOptions(targetPath string) WriteOptions {
	ext := filepath.Ext(targetPath)
	pattern := ".glance-tmp-*"
	if ext != "" {
		pattern = ".glance-tmp-*" + ext
	}
	return WriteOptions{
		Perm:               0644,
		TempPattern:        pattern,
		TempDir:            DefaultTempDir(),
		AutoCreateDir:      false,
		PreserveTargetPerm: true,
	}
}

// AtomicWrite writes to targetPath atomically via a sibling temporary file.
// The content is produced by calling writeFn.
func AtomicWrite(targetPath string, writeFn func(w io.Writer) error, opts ...WriteOption) error {
	if targetPath == "" {
		return errors.New("target path is empty")
	}
	if writeFn == nil {
		return errors.New("write function is required")
	}

	options := defaultWriteOptions(targetPath)
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}

	targetDir := filepath.Dir(targetPath)
	if options.AutoCreateDir {
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			return fmt.Errorf("create target directory %q: %w", targetDir, err)
		}
	}

	// Capture target file permissions if existing and requested
	perm := options.Perm
	if options.PreserveTargetPerm {
		if info, err := os.Stat(targetPath); err == nil {
			perm = info.Mode().Perm()
		}
	}

	tempDir := options.TempDir
	if tempDir == "" {
		tempDir = DefaultTempDir()
	}
	_ = os.MkdirAll(tempDir, 0700)

	tmpFile, err := os.CreateTemp(tempDir, options.TempPattern)
	if err != nil {
		return fmt.Errorf("create temporary file in %q: %w", tempDir, err)
	}
	tmpPath := tmpFile.Name()
	var committed bool
	defer func() {
		if !committed {
			_ = os.Remove(tmpPath)
		}
	}()

	// Produce content
	if err := writeFn(tmpFile); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("write temporary content: %w", err)
	}

	// Sync to ensure all OS page caches are flushed to persistent media
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("sync temporary file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temporary file: %w", err)
	}

	// Apply permissions
	if perm != 0 {
		_ = os.Chmod(tmpPath, perm)
	}

	// Execute pre-commit guard if configured
	var release func()
	if options.CommitGuard != nil {
		release, err = options.CommitGuard()
		if err != nil {
			return fmt.Errorf("commit guard rejected save: %w", err)
		}
	}
	if options.FileCommitGuard != nil {
		fileRelease, fileErr := options.FileCommitGuard(tmpPath)
		if fileErr != nil {
			if release != nil {
				release()
			}
			return fmt.Errorf("file commit guard rejected save: %w", fileErr)
		}
		if fileRelease != nil {
			prevRelease := release
			release = func() {
				if prevRelease != nil {
					prevRelease()
				}
				fileRelease()
			}
		}
	}
	if release != nil {
		defer release()
	}

	// Atomically replace target with temp file
	if err := ReplaceFile(tmpPath, targetPath); err != nil {
		return fmt.Errorf("replace target file %q: %w", targetPath, err)
	}
	committed = true
	return nil
}

// AtomicWriteBytes atomically writes a byte slice to targetPath.
func AtomicWriteBytes(targetPath string, data []byte, opts ...WriteOption) error {
	return AtomicWrite(targetPath, func(w io.Writer) error {
		_, err := w.Write(data)
		return err
	}, opts...)
}

// AtomicWriteString atomically writes a string to targetPath.
func AtomicWriteString(targetPath string, text string, opts ...WriteOption) error {
	return AtomicWrite(targetPath, func(w io.Writer) error {
		_, err := io.WriteString(w, text)
		return err
	}, opts...)
}

// AtomicWriteReader atomically writes from an io.Reader to targetPath using pooled stream buffers.
func AtomicWriteReader(targetPath string, r io.Reader, opts ...WriteOption) (int64, error) {
	var written int64
	err := AtomicWrite(targetPath, func(w io.Writer) error {
		var copyErr error
		written, copyErr = Copy(w, r)
		return copyErr
	}, opts...)
	return written, err
}
