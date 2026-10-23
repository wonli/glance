package builtin

import (
	"bytes"
	"encoding/binary"
	"fmt"

	fontapi "github.com/go-text/typesetting/font"
	fontopentype "github.com/go-text/typesetting/font/opentype"
)

// normalizeEmojiSpaceAdvance gives the bundled emoji face a conventional
// proportional space. Some shapers keep whitespace in the preceding glyph
// run, so the upstream wide U+0020 metric otherwise creates a large gap before
// adjacent text. Only the space advance and SFNT checksums are changed.
func normalizeEmojiSpaceAdvance(src []byte) ([]byte, error) {
	if len(src) < 12 {
		return nil, fmt.Errorf("font is shorter than an sfnt header")
	}
	loader, err := fontopentype.NewLoader(bytes.NewReader(src))
	if err != nil {
		return nil, err
	}
	parsed, err := fontapi.NewFont(loader)
	if err != nil {
		return nil, err
	}
	spaceGlyph, ok := parsed.Cmap.Lookup(' ')
	if !ok {
		return nil, fmt.Errorf("font has no U+0020 space glyph")
	}

	tables, err := sfntTables(src)
	if err != nil {
		return nil, err
	}
	head, hasHead := tables["head"]
	hhea, hasHhea := tables["hhea"]
	hmtx, hasHmtx := tables["hmtx"]
	if !hasHead || !hasHhea || !hasHmtx || head.length < 20 || hhea.length < 36 {
		return nil, fmt.Errorf("font is missing required head, hhea, or hmtx metrics")
	}
	unitsPerEm := binary.BigEndian.Uint16(src[head.offset+18 : head.offset+20])
	metricCount := binary.BigEndian.Uint16(src[hhea.offset+34 : hhea.offset+36])
	if unitsPerEm == 0 || int(spaceGlyph) >= int(metricCount) || int(spaceGlyph)*4+4 > hmtx.length {
		return nil, fmt.Errorf("space glyph %d has no independent horizontal metric", spaceGlyph)
	}
	// 300/1000 em is a conventional proportional space width, close to common
	// Latin body faces and independent of the raster ppem used for emoji bitmaps.
	spaceAdvance := uint16(uint32(unitsPerEm) * 300 / 1000)
	if spaceAdvance == 0 {
		return nil, fmt.Errorf("font units per em %d produce a zero-width space", unitsPerEm)
	}

	binary.BigEndian.PutUint16(src[hmtx.offset+int(spaceGlyph)*4:], spaceAdvance)
	updateSFNTChecksums(src, head, hmtx)
	return src, nil
}

type sfntTable struct {
	record int
	offset int
	length int
}

func sfntTables(data []byte) (map[string]sfntTable, error) {
	numTables := int(binary.BigEndian.Uint16(data[4:6]))
	if numTables == 0 || 12+numTables*16 > len(data) {
		return nil, fmt.Errorf("invalid sfnt table directory")
	}
	tables := make(map[string]sfntTable, numTables)
	for i := 0; i < numTables; i++ {
		record := 12 + i*16
		tag := string(data[record : record+4])
		offset := int(binary.BigEndian.Uint32(data[record+8 : record+12]))
		length := int(binary.BigEndian.Uint32(data[record+12 : record+16]))
		if offset < 0 || length < 0 || offset > len(data) || length > len(data)-offset {
			return nil, fmt.Errorf("invalid %q table bounds", tag)
		}
		tables[tag] = sfntTable{record: record, offset: offset, length: length}
	}
	return tables, nil
}

func updateSFNTChecksums(data []byte, head, hmtx sfntTable) {
	// The head table checksum is defined with checkSumAdjustment set to zero.
	binary.BigEndian.PutUint32(data[head.offset+8:head.offset+12], 0)
	binary.BigEndian.PutUint32(data[head.record+4:head.record+8], sfntChecksum(data[head.offset:head.offset+head.length]))
	binary.BigEndian.PutUint32(data[hmtx.record+4:hmtx.record+8], sfntChecksum(data[hmtx.offset:hmtx.offset+hmtx.length]))
	const wholeFontChecksum = uint32(0xB1B0AFBA)
	adjustment := wholeFontChecksum - sfntChecksum(data)
	binary.BigEndian.PutUint32(data[head.offset+8:head.offset+12], adjustment)
}

func sfntChecksum(data []byte) uint32 {
	var sum uint32
	for len(data) >= 4 {
		sum += binary.BigEndian.Uint32(data[:4])
		data = data[4:]
	}
	if len(data) > 0 {
		var tail [4]byte
		copy(tail[:], data)
		sum += binary.BigEndian.Uint32(tail[:])
	}
	return sum
}
