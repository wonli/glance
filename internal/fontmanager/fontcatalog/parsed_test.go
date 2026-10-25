package fontcatalog

import (
	"path/filepath"
	"testing"
)

func TestSnapshotOpenParsedFace(t *testing.T) {
	emojiPath, err := filepath.Abs("../builtin/assets/NotoColorEmoji.ttf")
	if err != nil {
		t.Fatal(err)
	}

	snapshot := NewSnapshot(1, []Face{
		{
			ID:     "emoji-face-0",
			Family: "Noto Color Emoji",
			Path:   emojiPath,
			Index:  0,
			Weight: 400,
		},
	})

	f1, err := snapshot.OpenParsedFace("emoji-face-0")
	if err != nil {
		t.Fatalf("OpenParsedFace failed: %v", err)
	}
	if f1 == nil {
		t.Fatal("OpenParsedFace returned nil font")
	}

	// Calling a second time should return identical cached pointer.
	f2, err := snapshot.OpenParsedFace("emoji-face-0")
	if err != nil {
		t.Fatalf("OpenParsedFace (second call) failed: %v", err)
	}
	if f1 != f2 {
		t.Fatalf("OpenParsedFace pointer mismatch: %p != %p; want deduplicated instance", f1, f2)
	}
}
