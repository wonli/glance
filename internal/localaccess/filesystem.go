package localaccess

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

var (
	ErrInvalidRelativePath = errors.New("path must be a safe relative path")
	ErrPathOutsideScope    = errors.New("path is outside the authorized directory")
	ErrSensitiveFile       = errors.New("file may contain credentials or secrets")
	ErrUnsupportedFile     = errors.New("file is not supported as UTF-8 text")
	ErrFileTooLarge        = errors.New("file exceeds the 1 MiB read limit")
	ErrDirectoryEntryLimit = errors.New("directory exceeds the 500-entry listing limit")
)

const (
	maxDirectoryEntries = 500
	maxTextFileBytes    = 1 << 20
)

func (b *Broker) ListDirectory(ctx context.Context, turnID, scopeID, relativePath string) ([]Entry, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	scope, state, err := b.lookupScope(turnID, scopeID)
	if err != nil {
		return nil, err
	}
	if err := scope.validateRoot(); err != nil {
		return nil, err
	}
	clean, err := cleanRelativePath(relativePath)
	if err != nil {
		return nil, err
	}
	directory, err := resolveWithinScope(scope.root, clean)
	if err != nil {
		return nil, sanitizePathError(err)
	}
	if isSensitivePath(clean) || isSensitiveResolvedPath(scope.root, directory) {
		return nil, ErrSensitiveFile
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := scope.validateRoot(); err != nil {
		return nil, err
	}
	file, err := openFileNoBlock(directory)
	if err != nil {
		return nil, ErrInvalidRelativePath
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil || !openedInfo.IsDir() {
		return nil, ErrInvalidRelativePath
	}
	currentInfo, err := os.Stat(directory)
	if err != nil || !os.SameFile(openedInfo, currentInfo) {
		return nil, ErrPathOutsideScope
	}
	entries, err := file.ReadDir(maxDirectoryEntries + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, ErrInvalidRelativePath
	}
	if len(entries) > maxDirectoryEntries {
		return nil, ErrDirectoryEntryLimit
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	result := make([]Entry, 0, min(len(entries), maxDirectoryEntries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		name := entry.Name()
		if name == ".git" || isSensitivePath(name) {
			continue
		}
		childRelative := name
		if clean != "." {
			childRelative = filepath.Join(clean, name)
		}
		childPath, err := resolveWithinScope(scope.root, childRelative)
		if err != nil {
			// Do not expose links which point outside the granted directory.
			if errors.Is(err, ErrPathOutsideScope) || errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, sanitizePathError(err)
		}
		if isSensitiveResolvedPath(scope.root, childPath) {
			continue
		}
		info, err := os.Stat(childPath)
		if err != nil {
			continue
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			continue
		}
		result = append(result, Entry{
			Name:         name,
			RelativePath: filepath.ToSlash(childRelative),
			IsDirectory:  info.IsDir(),
			Size:         info.Size(),
		})
		if len(result) == maxDirectoryEntries {
			break
		}
	}
	if !b.isActiveTurn(turnID, state) {
		return nil, ErrTurnExpired
	}
	if err := scope.validateRoot(); err != nil {
		return nil, err
	}
	return result, nil
}

func (b *Broker) ReadTextFile(ctx context.Context, turnID, scopeID, relativePath string) (TextFile, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	scope, state, err := b.lookupScope(turnID, scopeID)
	if err != nil {
		return TextFile{}, err
	}
	if err := scope.validateRoot(); err != nil {
		return TextFile{}, err
	}
	clean, err := cleanRelativePath(relativePath)
	if err != nil {
		return TextFile{}, err
	}
	if clean == "." {
		return TextFile{}, ErrUnsupportedFile
	}
	if err := ctx.Err(); err != nil {
		return TextFile{}, err
	}
	resolved, err := resolveWithinScope(scope.root, clean)
	if err != nil {
		return TextFile{}, sanitizePathError(err)
	}
	if isSensitivePath(clean) || isSensitiveResolvedPath(scope.root, resolved) {
		return TextFile{}, ErrSensitiveFile
	}
	if err := scope.validateRoot(); err != nil {
		return TextFile{}, err
	}
	pathInfo, err := os.Stat(resolved)
	if err != nil || !pathInfo.Mode().IsRegular() {
		return TextFile{}, ErrUnsupportedFile
	}
	file, err := openFileNoBlock(resolved)
	if err != nil {
		return TextFile{}, ErrInvalidRelativePath
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil || !openedInfo.Mode().IsRegular() {
		return TextFile{}, ErrUnsupportedFile
	}
	currentPath, err := filepath.EvalSymlinks(resolved)
	if err != nil || !withinRoot(scope.root, currentPath) {
		return TextFile{}, ErrPathOutsideScope
	}
	currentInfo, err := os.Stat(currentPath)
	if err != nil || !os.SameFile(openedInfo, currentInfo) {
		return TextFile{}, ErrPathOutsideScope
	}
	if openedInfo.Size() > maxTextFileBytes {
		return TextFile{}, ErrFileTooLarge
	}
	content, err := io.ReadAll(io.LimitReader(file, maxTextFileBytes+1))
	if err != nil {
		return TextFile{}, ErrUnsupportedFile
	}
	if len(content) > maxTextFileBytes {
		return TextFile{}, ErrFileTooLarge
	}
	if !utf8.Valid(content) || strings.IndexByte(string(content), 0) >= 0 {
		return TextFile{}, ErrUnsupportedFile
	}
	if containsSensitiveContent(string(content)) {
		return TextFile{}, ErrSensitiveFile
	}
	if err := ctx.Err(); err != nil {
		return TextFile{}, err
	}
	if !b.isActiveTurn(turnID, state) {
		return TextFile{}, ErrTurnExpired
	}
	if err := scope.validateRoot(); err != nil {
		return TextFile{}, err
	}
	return TextFile{RelativePath: filepath.ToSlash(clean), Content: string(content), Size: int64(len(content))}, nil
}

func (b *Broker) lookupScope(turnID, scopeID string) (*folderScope, *turnState, error) {
	if b == nil || strings.TrimSpace(turnID) == "" || strings.TrimSpace(scopeID) == "" {
		return nil, nil, ErrInvalidScope
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, nil, ErrBrokerClosed
	}
	state := b.turns[turnID]
	if state == nil {
		if _, expired := b.retired[turnID]; expired {
			return nil, nil, ErrTurnExpired
		}
		return nil, nil, ErrInvalidScope
	}
	state.mu.RLock()
	scope := state.scopes[scopeID]
	state.mu.RUnlock()
	if scope == nil {
		return nil, nil, ErrInvalidScope
	}
	return scope, state, nil
}

func cleanRelativePath(path string) (string, error) {
	if path == "" || strings.IndexByte(path, 0) >= 0 || strings.Contains(path, `\`) || filepath.IsAbs(path) || filepath.VolumeName(path) != "" {
		return "", ErrInvalidRelativePath
	}
	if path == "." {
		return ".", nil
	}
	for _, component := range strings.Split(path, "/") {
		if component == "" || component == "." || component == ".." {
			return "", ErrPathOutsideScope
		}
	}
	clean := filepath.Clean(filepath.FromSlash(path))
	if clean == "." || clean == ".." || filepath.IsAbs(clean) {
		return "", ErrPathOutsideScope
	}
	return clean, nil
}

func resolveWithinScope(root, relative string) (string, error) {
	candidate := root
	if relative != "." {
		candidate = filepath.Join(root, relative)
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", err
	}
	if !withinRoot(root, resolved) {
		return "", ErrPathOutsideScope
	}
	return resolved, nil
}

func withinRoot(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

func isSensitivePath(path string) bool {
	for _, component := range strings.Split(strings.ToLower(filepath.ToSlash(path)), "/") {
		base := strings.TrimSpace(component)
		switch base {
		case ".git", ".ssh", ".aws", ".gnupg", ".kube", ".netrc", ".npmrc", "id_rsa", "id_ed25519", "id_ecdsa", "credentials", "credentials.json", "secrets", "secrets.json":
			return true
		}
		if strings.HasPrefix(base, ".env") || strings.HasPrefix(base, "credentials.") || strings.HasPrefix(base, "secret.") || strings.HasSuffix(base, ".pem") || strings.HasSuffix(base, ".p12") || strings.HasSuffix(base, ".pfx") || strings.HasSuffix(base, ".key") {
			return true
		}
	}
	return false
}

func isSensitiveResolvedPath(root, resolved string) bool {
	relative, err := filepath.Rel(root, resolved)
	if err != nil || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return true
	}
	return isSensitivePath(filepath.ToSlash(relative))
}

func sanitizePathError(err error) error {
	if errors.Is(err, ErrPathOutsideScope) {
		return ErrPathOutsideScope
	}
	return ErrInvalidRelativePath
}

func containsSensitiveContent(content string) bool {
	lower := strings.ToLower(content)
	for _, marker := range []string{
		"-----begin private key-----",
		"-----begin rsa private key-----",
		"-----begin ec private key-----",
		"-----begin dsa private key-----",
		"-----begin openssh private key-----",
		"-----begin encrypted private key-----",
		"-----begin pgp private key block-----",
		"putty-user-key-file-",
		"aws_access_key_id=",
		"aws_secret_access_key=",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}
