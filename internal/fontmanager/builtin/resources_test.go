package builtin

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"testing"

	fontapi "github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/font/opentype"
)

func TestBundledEmojiProvenance(t *testing.T) {
	raw, err := assets.ReadFile("assets/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Files map[string]struct {
			SHA256 string `json:"sha256"`
		} `json:"files"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"NotoColorEmoji.ttf", "OFL.txt"} {
		data, err := assets.ReadFile("assets/" + name)
		if err != nil {
			t.Fatal(err)
		}
		sum := fmt.Sprintf("%x", sha256.Sum256(data))
		if sum != manifest.Files[name].SHA256 {
			t.Fatalf("digest mismatch for %s", name)
		}
	}
}

func TestEmojiFontNormalizesSpaceWithoutChangingColorGlyphs(t *testing.T) {
	original, err := assets.ReadFile("assets/NotoColorEmoji.ttf")
	if err != nil {
		t.Fatal(err)
	}
	normalized, err := EmojiFont()
	if err != nil {
		t.Fatal(err)
	}
	if got := sfntChecksum(normalized); got != 0xB1B0AFBA {
		t.Fatalf("normalized sfnt checksum=%08x, want %08x", got, uint32(0xB1B0AFBA))
	}
	parseFace := func(data []byte) *fontapi.Face {
		t.Helper()
		loader, err := opentype.NewLoader(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := fontapi.NewFont(loader)
		if err != nil {
			t.Fatal(err)
		}
		return fontapi.NewFace(parsed)
	}
	originalFace, normalizedFace := parseFace(original), parseFace(normalized)
	spaceGlyph, ok := normalizedFace.NominalGlyph(' ')
	if !ok {
		t.Fatal("bundled emoji has no U+0020 space glyph")
	}
	tables, err := sfntTables(normalized)
	if err != nil {
		t.Fatal(err)
	}
	head := tables["head"]
	unitsPerEm := binary.BigEndian.Uint16(normalized[head.offset+18 : head.offset+20])
	wantAdvance := float32(uint32(unitsPerEm) * 300 / 1000)
	if got := normalizedFace.HorizontalAdvance(spaceGlyph); got != wantAdvance {
		t.Fatalf("normalized space advance=%v font units, want %v", got, wantAdvance)
	}
	if got := originalFace.HorizontalAdvance(spaceGlyph); got <= wantAdvance {
		t.Fatalf("upstream space advance=%v font units, want it wider than normalized %v", got, wantAdvance)
	}
	for _, r := range "🚀⚠" {
		gid, ok := normalizedFace.NominalGlyph(r)
		if !ok {
			t.Fatalf("missing color glyph %c", r)
		}
		before, beforeOK := originalFace.GlyphData(gid).(fontapi.GlyphBitmap)
		after, afterOK := normalizedFace.GlyphData(gid).(fontapi.GlyphBitmap)
		if !beforeOK || !afterOK || !bytes.Equal(before.Data, after.Data) {
			t.Fatalf("normalizing space changed color bitmap for %c", r)
		}
	}

	// Consumers may modify their returned copy without affecting the manager's
	// process-wide immutable resource.
	normalized[0] ^= 0xff
	second, err := EmojiFont()
	if err != nil {
		t.Fatal(err)
	}
	if second[0] == normalized[0] {
		t.Fatal("mutating a returned font copy changed the cached manager resource")
	}
}
