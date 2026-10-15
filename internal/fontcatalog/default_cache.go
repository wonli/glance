package fontcatalog

import (
	"context"
	"os"
	"path/filepath"
)

func DefaultPaths() (cachePath, objectsPath string, err error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", "", err
	}
	data, err := os.UserConfigDir()
	if err != nil {
		return "", "", err
	}
	return filepath.Join(cache, "glance", "fonts", "catalog-v1.json"), filepath.Join(data, "glance", "fonts", "objects"), nil
}

// LoadResources supports headless rendering and recovery after cache eviction.
func LoadResources(ctx context.Context, cachePath, objectsPath string, directories []string) (Snapshot, error) {
	if cached, err := LoadCache(cachePath); err == nil {
		return cached, nil
	}
	scanner := Scanner{Directories: directories}
	if info, err := os.Stat(objectsPath); err == nil && info.IsDir() {
		scanner.PrivateDirectories = []string{objectsPath}
	}
	result, err := scanner.Scan(ctx, Snapshot{}, false)
	if err != nil {
		return Snapshot{}, err
	}
	return NewSnapshot(1, result.Faces), nil
}
