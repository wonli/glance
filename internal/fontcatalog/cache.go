package fontcatalog

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

const cacheSchema = 1
const parserVersion = "go-text-0.3.5"

type diskIndex struct {
	Schema    int
	Platform  string
	Parser    string
	Version   uint64
	CheckedAt time.Time
	Faces     []Face
}

func LoadCache(path string) (Snapshot, error) {
	file, err := os.Open(path)
	if err != nil {
		return Snapshot{}, err
	}
	defer file.Close()
	var index diskIndex
	if err := json.NewDecoder(io.LimitReader(file, 32<<20)).Decode(&index); err != nil {
		return Snapshot{}, err
	}
	if index.Schema != cacheSchema || index.Platform != runtime.GOOS || index.Parser != parserVersion {
		return Snapshot{}, errors.New("incompatible font index")
	}
	for _, f := range index.Faces {
		if f.ID == "" || f.Family == "" || f.Path == "" || f.Digest == "" {
			return Snapshot{}, errors.New("invalid cached font identity")
		}
	}
	snapshot := NewSnapshot(index.Version, index.Faces)
	snapshot.CheckedAt = index.CheckedAt
	return snapshot, nil
}
func SaveCache(path string, snapshot Snapshot) error {
	data, err := json.Marshal(diskIndex{Schema: cacheSchema, Platform: runtime.GOOS, Parser: parserVersion, Version: snapshot.Version, CheckedAt: snapshot.CheckedAt, Faces: snapshot.Faces()})
	if err != nil {
		return err
	}
	return WriteAtomic(path, data)
}

// WriteAtomic publishes only a complete file, keeping its temporary sibling private.
func WriteAtomic(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".font-stage-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
