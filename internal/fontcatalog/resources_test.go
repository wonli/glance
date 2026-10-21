package fontcatalog

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestOpenFaceResourceStreamsLargeFile(t *testing.T) {
	const size = 32 << 20
	path := filepath.Join(t.TempDir(), "large-font.bin")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(size); err != nil {
		file.Close()
		t.Fatal(err)
	}
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	snapshot := NewSnapshot(1, []Face{{
		ID: "large", Path: path, Digest: hex.EncodeToString(h.Sum(nil)), Size: size,
	}})
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	resource, err := snapshot.OpenFaceResource("large")
	if err != nil {
		t.Fatal(err)
	}
	if err := resource.Close(); err != nil {
		t.Fatal(err)
	}
	runtime.ReadMemStats(&after)

	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 4<<20 {
		t.Fatalf("opening a %d-byte font resource allocated %d bytes; want under 4 MiB", size, allocated)
	}
}

func TestOpenFaceResourceRejectsChangedContents(t *testing.T) {
	path := testFont(t, "changed.ttf")
	faces, err := Inspect(t.Context(), path, System)
	if err != nil || len(faces) != 1 {
		t.Fatalf("inspect font: faces=%d err=%v", len(faces), err)
	}
	snapshot := NewSnapshot(1, faces)
	if err := os.WriteFile(path, []byte("changed font bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	if resource, err := snapshot.OpenFaceResource(faces[0].ID); err == nil {
		resource.Close()
		t.Fatal("opened a font whose contents no longer match the snapshot digest")
	}
}
