package doc

import (
	"encoding/binary"
	"glance/internal/document"
	"strings"
	"testing"
	"unicode/utf16"
)

func nativeLayoutFixture() []byte {
	w := make([]byte, 4096)
	table := make([]byte, 4096)
	put16 := func(data []byte, at int, v uint16) { binary.LittleEndian.PutUint16(data[at:], v) }
	put32 := func(data []byte, at int, v uint32) { binary.LittleEndian.PutUint32(data[at:], v) }
	put16(w, 0, 0xa5ec)
	put16(w, 2, 0xc1)
	put16(w, 32, 14)
	put16(w, 62, 22)
	put16(w, 152, 93)
	body := utf16.Encode([]rune("正文\r签章\r"))
	header := utf16.Encode([]rune("\x13PAGE \x141\x15\r\r"))
	chars := append(append(append([]uint16{}, body...), header...), 13)
	put32(w, 76, uint32(len(body)))
	put32(w, 84, uint32(len(header)))
	for i, c := range chars {
		put16(w, 2048+2*i, c)
	}
	setPart := func(index, offset, length int) {
		put32(w, 154+8*index, uint32(offset))
		put32(w, 154+8*index+4, uint32(length))
	}
	put32(table, 0, 0)
	put32(table, 4, uint32(len(chars)))
	put32(table, 10, 3072)
	setPart(6, 0, 20)
	var props []byte
	for _, p := range [][2]uint16{{0xb01f, 11906}, {0xb020, 16838}, {0xb021, 1800}, {0xb022, 1800}, {0x9023, 1440}, {0x9024, 1440}, {0x5032, 2}, {0x9031, 312}} {
		props = append(props, byte(p[0]), byte(p[0]>>8), byte(p[1]), byte(p[1]>>8))
	}
	put16(w, 3072, uint16(len(props)))
	copy(w[3074:], props)
	table[32] = 2
	put32(table, 33, 16)
	put32(table, 37, 0)
	put32(table, 41, uint32(len(chars)))
	put32(table, 47, 2048)
	setPart(33, 32, 21)
	for i := 0; i < 13; i++ {
		v := uint32(0)
		if i >= 10 {
			v = uint32(len(header))
		}
		put32(table, 64+4*i, v)
	}
	setPart(11, 64, 52)
	return compoundFixture(w, table)
}

func compoundFixture(word, table []byte) []byte {
	b := make([]byte, 19*512)
	copy(b, compoundFileSignature[:])
	put16 := func(at int, v uint16) { binary.LittleEndian.PutUint16(b[at:], v) }
	put32 := func(at int, v uint32) { binary.LittleEndian.PutUint32(b[at:], v) }
	put16(26, 3)
	put16(28, 0xfffe)
	put16(30, 9)
	put16(32, 6)
	put32(44, 1)
	put32(48, 0)
	put32(56, 4096)
	put32(60, compoundEnd)
	put32(68, compoundEnd)
	for i := 0; i < 109; i++ {
		put32(76+4*i, compoundFree)
	}
	put32(76, 17)
	dir := func(index int, name string, kind byte, start, size uint32) {
		at := 512 + 128*index
		u := utf16.Encode([]rune(name))
		for i, c := range u {
			put16(at+2*i, c)
		}
		put16(at+64, uint16(2*(len(u)+1)))
		b[at+66] = kind
		put32(at+68, compoundFree)
		put32(at+72, compoundFree)
		put32(at+76, compoundFree)
		put32(at+116, start)
		put32(at+120, size)
	}
	dir(0, "Root Entry", 5, compoundEnd, 0)
	dir(1, "WordDocument", 2, 1, 4096)
	dir(2, "0Table", 2, 9, 4096)
	put32(512+76, 1)
	put32(512+128+72, 2)
	copy(b[1024:5120], word)
	copy(b[5120:9216], table)
	for i := 0; i < 128; i++ {
		put32(9216+4*i, compoundFree)
	}
	put32(9216, compoundEnd)
	for i := 1; i <= 16; i++ {
		next := uint32(i + 1)
		if i == 8 || i == 16 {
			next = compoundEnd
		}
		put32(9216+4*i, next)
	}
	put32(9216+4*17, 0xfffffffd)
	return b
}

func TestNativeLayoutReadsGridAndStoryBoundaryWithoutManualPageBreak(t *testing.T) {
	l, err := nativeImportLayout(nativeLayoutFixture())
	if err != nil {
		t.Fatal(err)
	}
	if l == nil || l.Page.WidthPT != 595.3 || l.Page.HeightPT != 841.9 || l.Grid.Type != "lines" || l.Grid.LinePitchPT != 15.6 {
		t.Fatalf("recovered layout=%+v", l)
	}
	if len(l.Paragraphs) != 2 || l.Paragraphs[1].Text != "签章" || strings.Contains(l.Paragraphs[0].Text, "PAGE") {
		t.Fatalf("main story=%+v", l.Paragraphs)
	}
	if len(l.Stories) != 1 || l.Stories[0].Kind != "footer" || l.Stories[0].Type != "default" || !strings.Contains(l.Stories[0].Text, "\x13PAGE") {
		t.Fatalf("footer story=%+v", l.Stories)
	}
}

func TestNativeLayoutRejectsCorruptOffsetsAndCyclicStorage(t *testing.T) {
	tests := map[string]func([]byte){
		"directory-cycle":        func(b []byte) { binary.LittleEndian.PutUint32(b[9216:], 0) },
		"piece-outside-stream":   func(b []byte) { binary.LittleEndian.PutUint32(b[5120+47:], 0x3ffffffe) },
		"section-outside-stream": func(b []byte) { binary.LittleEndian.PutUint32(b[5120+10:], 0x7fffffff) },
		"encrypted":              func(b []byte) { binary.LittleEndian.PutUint16(b[1024+10:], 0x100) },
		"excess-fib-table":       func(b []byte) { binary.LittleEndian.PutUint16(b[1024+152:], 0xffff) },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			b := nativeLayoutFixture()
			mutate(b)
			if _, err := nativeImportLayout(b); err == nil {
				t.Fatal("corrupt Word binary metadata was accepted")
			}
		})
	}
}

func TestNativeLayoutRetainsSystemConversionForOtherDocumentStories(t *testing.T) {
	b := nativeLayoutFixture()
	binary.LittleEndian.PutUint32(b[1024+80:], 2)
	l, err := nativeImportLayout(b)
	if err != nil {
		t.Fatal(err)
	}
	if l != nil {
		t.Fatal("footnote document would be omitted by plain-story repair")
	}
}

func TestNativeLayoutRetainsManualAndParagraphPageBreaks(t *testing.T) {
	b := nativeLayoutFixture()
	// An inline U+000C remains a manual break in the source text.
	binary.LittleEndian.PutUint16(b[1024+2048:], 12)
	// Paragraph formatting for the second paragraph sets pageBreakBefore.
	w := b[1024:5120]
	table := b[5120:9216]
	binary.LittleEndian.PutUint32(w[154+13*8:], 128)
	binary.LittleEndian.PutUint32(w[154+13*8+4:], 12)
	binary.LittleEndian.PutUint32(table[128:], 2048)
	binary.LittleEndian.PutUint32(table[132:], 2060)
	binary.LittleEndian.PutUint32(table[136:], 7)
	fkp := w[3584:4096]
	binary.LittleEndian.PutUint32(fkp, 2048)
	binary.LittleEndian.PutUint32(fkp[4:], 2054)
	binary.LittleEndian.PutUint32(fkp[8:], 2060)
	fkp[25], fkp[500], fkp[501], fkp[504], fkp[505], fkp[506], fkp[511] = 250, 0, 3, 7, 0x24, 1, 2
	l, err := nativeImportLayout(b)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(l.Paragraphs[0].Text, "\f") || !l.Paragraphs[1].Layout.PageBreakBefore {
		t.Fatal("explicit page breaks were lost in native DOC metadata")
	}
}

func TestCompoundWordStreamsIgnoreNestedStorageNames(t *testing.T) {
	original := nativeLayoutFixture()
	b := append(append([]byte{}, original...), make([]byte, 512)...)
	binary.LittleEndian.PutUint32(b[9216:], 18)
	binary.LittleEndian.PutUint32(b[9216+18*4:], compoundEnd)
	binary.LittleEndian.PutUint32(b[512+2*128+72:], 3)
	storage := b[512+3*128 : 512+4*128]
	storage[66] = 1
	for _, at := range []int{68, 72} {
		binary.LittleEndian.PutUint32(storage[at:], compoundFree)
	}
	binary.LittleEndian.PutUint32(storage[76:], 4)
	for i, name := range []string{"WordDocument", "0Table"} {
		d := b[9728+i*128 : 9728+(i+1)*128]
		chars := utf16.Encode([]rune(name))
		for j, c := range chars {
			binary.LittleEndian.PutUint16(d[j*2:], c)
		}
		binary.LittleEndian.PutUint16(d[64:], uint16(2*(len(chars)+1)))
		d[66] = 2
		for _, at := range []int{68, 72, 76} {
			binary.LittleEndian.PutUint32(d[at:], compoundFree)
		}
		binary.LittleEndian.PutUint32(d[116:], compoundEnd)
		if i == 0 {
			binary.LittleEndian.PutUint32(d[72:], 5)
		}
	}
	word, table, err := readCompoundWordStreams(b)
	if err != nil || len(word) != 4096 || len(table) != 4096 {
		t.Fatalf("root streams: word=%d table=%d err=%v", len(word), len(table), err)
	}
}

func TestNativeLayoutRecoversUniformParagraphScriptFonts(t *testing.T) {
	b := nativeLayoutFixture()
	w := b[1024:5120]
	table := b[5120:9216]
	names := []string{"Times New Roman", "黑体"}
	fonts := []byte{2, 0, 0, 0}
	for _, name := range names {
		u := append(utf16.Encode([]rune(name)), 0)
		f := make([]byte, 40+2*len(u))
		f[0] = byte(len(f) - 1)
		for i, c := range u {
			binary.LittleEndian.PutUint16(f[40+2*i:], c)
		}
		fonts = append(fonts, f...)
	}
	copy(table[256:], fonts)
	binary.LittleEndian.PutUint32(w[154+15*8:], 256)
	binary.LittleEndian.PutUint32(w[154+15*8+4:], uint32(len(fonts)))
	binary.LittleEndian.PutUint32(w[154+12*8:], 128)
	binary.LittleEndian.PutUint32(w[154+12*8+4:], 12)
	binary.LittleEndian.PutUint32(table[128:], 2048)
	binary.LittleEndian.PutUint32(table[132:], 2060)
	binary.LittleEndian.PutUint32(table[136:], 7)
	fkp := w[3584:]
	binary.LittleEndian.PutUint32(fkp, 2048)
	binary.LittleEndian.PutUint32(fkp[4:], 2060)
	fkp[8] = 240
	fkp[480] = 8
	copy(fkp[481:], []byte{0x4f, 0x4a, 1, 0, 0x50, 0x4a, 1, 0})
	fkp[511] = 1
	l, err := nativeImportLayout(b)
	if err != nil {
		t.Fatal(err)
	}
	if l.Paragraphs[0].Fonts.FontFamilyASCII != "黑体" || l.Paragraphs[0].Fonts.FontFamilyEastAsia != "黑体" {
		t.Fatalf("source fonts=%+v", l.Paragraphs[0].Fonts)
	}
}

func TestUniformParagraphFontsDeclinesMixedAndUnmappedRuns(t *testing.T) {
	ranges := []nativeFontRange{{start: 100, end: 102, fonts: document.WordTextStyle{FontFamilyASCII: "黑体"}}, {start: 102, end: 104, fonts: document.WordTextStyle{FontFamilyASCII: "宋体"}}}
	for _, positions := range [][]uint32{{100, 102}, {99}, {104}} {
		if got := uniformParagraphFonts(positions, ranges); got != (document.WordTextStyle{}) {
			t.Fatalf("unsupported source mapping repaired: %+v", got)
		}
	}
}
