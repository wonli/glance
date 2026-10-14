package doc

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"unicode/utf16"

	"glance/internal/document"
	"glance/internal/format/docx"
	"golang.org/x/text/encoding/charmap"
)

type nativeWord struct {
	word, table, fib       []byte
	body, footnote, header uint32
}

func binarySpan(data []byte, offset, size uint32) ([]byte, error) {
	end := uint64(offset) + uint64(size)
	if end > uint64(len(data)) {
		return nil, errors.New("Word binary range outside stream")
	}
	return data[int(offset):int(end)], nil
}
func wordU16(data []byte, at int) (uint16, error) {
	if at < 0 || at+2 > len(data) {
		return 0, errors.New("truncated Word binary integer")
	}
	return binary.LittleEndian.Uint16(data[at:]), nil
}

func nativeImportLayout(data []byte) (*docx.LegacyImportLayout, error) {
	w, t, err := readCompoundWordStreams(data)
	if err != nil {
		return nil, err
	}
	magic, err := wordU16(w, 0)
	if err != nil || magic != 0xa5ec {
		return nil, errors.New("invalid Word FIB signature")
	}
	version, _ := wordU16(w, 2)
	if version < 0xc1 {
		return nil, nil
	}
	flags, _ := wordU16(w, 10)
	if flags&0x100 != 0 {
		return nil, errors.New("encrypted Word documents cannot recover native layout")
	}
	csw, err := wordU16(w, 32)
	if err != nil {
		return nil, err
	}
	at := 34 + int(csw)*2
	cslw, err := wordU16(w, at)
	if err != nil || cslw < 11 {
		return nil, errors.New("truncated Word FIB character counts")
	}
	lw := at + 2
	base := lw + int(cslw)*4
	count, err := wordU16(w, base)
	if err != nil {
		return nil, err
	}
	base += 2
	if count < 34 || uint64(base)+uint64(count)*8 > uint64(len(w)) {
		return nil, errors.New("truncated Word FIB offset table")
	}
	n := nativeWord{word: w, table: t, fib: w[base : base+int(count)*8], body: binary.LittleEndian.Uint32(w[lw+12:]), footnote: binary.LittleEndian.Uint32(w[lw+16:]), header: binary.LittleEndian.Uint32(w[lw+20:])}
	// This recovery path can identify the plain main and header documents.
	// Other stories stay in the system conversion until their own mappings exist.
	for _, index := range []int{4, 7, 8, 9, 10} {
		if binary.LittleEndian.Uint32(w[lw+index*4:]) != 0 {
			return nil, nil
		}
	}
	sections, err := n.part(6)
	if err != nil {
		return nil, err
	}
	if len(sections) == 0 {
		return nil, nil
	}
	if len(sections) < 20 || (len(sections)-4)%16 != 0 {
		return nil, errors.New("invalid Word section PLC")
	}
	// Multiple sections need property inheritance and per-section story mapping.
	// Retain the system conversion for that currently unsupported recovery case.
	if (len(sections)-4)/16 != 1 {
		return nil, nil
	}
	sepx := binary.LittleEndian.Uint32(sections[10:])
	var group []byte
	if sepx != math.MaxUint32 {
		h, e := binarySpan(w, sepx, 2)
		if e != nil {
			return nil, e
		}
		group, e = binarySpan(w, sepx+2, uint32(binary.LittleEndian.Uint16(h)))
		if e != nil {
			return nil, e
		}
	}
	layout := &docx.LegacyImportLayout{}
	err = readWordPRLs(group, func(code uint16, v []byte) {
		if code == 0x301d && len(v) == 1 {
			layout.Page.Orientation = "portrait"
			if v[0] != 0 {
				layout.Page.Orientation = "landscape"
			}
			layout.Page.Set |= document.WordPageOrientationSet
		}
		if len(v) != 2 {
			return
		}
		value := float64(binary.LittleEndian.Uint16(v)) / 20
		switch code {
		case 0xb01f:
			layout.Page.WidthPT = value
			layout.Page.Set |= document.WordPageWidthSet
		case 0xb020:
			layout.Page.HeightPT = value
			layout.Page.Set |= document.WordPageHeightSet
		case 0xb021:
			layout.Page.MarginLeftPT = value
			layout.Page.Set |= document.WordPageMarginLeftSet
		case 0xb022:
			layout.Page.MarginRightPT = value
			layout.Page.Set |= document.WordPageMarginRightSet
		case 0x9023:
			layout.Page.MarginTopPT = float64(int16(binary.LittleEndian.Uint16(v))) / 20
			layout.Page.Set |= document.WordPageMarginTopSet
		case 0x9024:
			layout.Page.MarginBottomPT = float64(int16(binary.LittleEndian.Uint16(v))) / 20
			layout.Page.Set |= document.WordPageMarginBottomSet
		case 0xb017:
			layout.HeaderDistancePT = value
			layout.HeaderDistanceSet = true
		case 0xb018:
			layout.FooterDistancePT = value
			layout.FooterDistanceSet = true
		case 0x9031:
			layout.Grid.LinePitchPT = value
		case 0x5032:
			mode := binary.LittleEndian.Uint16(v)
			if mode < 4 {
				layout.Grid.Type = []string{"default", "linesandchars", "lines", "snaptochars"}[mode]
			}
		}
	})
	if err != nil {
		return nil, err
	}
	chars, fc, err := n.text()
	if err != nil {
		return nil, err
	}
	if uint64(n.body) > uint64(len(chars)) {
		return nil, errors.New("Word body character count exceeds piece table")
	}
	if n.body == 0 || chars[n.body-1] != 13 {
		return nil, errors.New("Word main story has no final paragraph mark")
	}
	fontRanges, err := n.fontRanges()
	if err != nil {
		return nil, err
	}
	for start, end := 0, 0; end < int(n.body); end++ {
		if chars[end] != 13 {
			continue
		}
		props, err := n.paragraphProperties(fc[end])
		if err != nil {
			return nil, err
		}
		layout.Paragraphs = append(layout.Paragraphs, docx.LegacyImportParagraph{Text: string(utf16.Decode(chars[start:end])), Layout: props, Fonts: uniformParagraphFonts(fc[start:end], fontRanges)})
		start = end + 1
	}
	hdd, err := n.part(11)
	if err != nil {
		return nil, err
	}
	if n.header > 0 {
		if len(hdd) < 52 {
			return nil, errors.New("truncated Word header-story PLC")
		}
		for at := 0; at < 24; at += 4 {
			if binary.LittleEndian.Uint32(hdd[at:]) != 0 {
				return nil, nil
			}
		}
	}
	headerStart := uint64(n.body) + uint64(n.footnote)
	if n.header > 0 && len(hdd) >= 52 && headerStart+uint64(n.header) <= uint64(len(chars)) {
		roles := []string{"header", "header", "footer", "footer", "header", "footer"}
		types := []string{"even", "default", "even", "default", "first", "first"}
		for i := 0; i < 6; i++ {
			start := binary.LittleEndian.Uint32(hdd[4*(6+i):])
			end := binary.LittleEndian.Uint32(hdd[4*(7+i):])
			if start >= end {
				continue
			}
			if end > n.header {
				return nil, errors.New("Word header story outside header document")
			}
			story := chars[int(headerStart)+int(start) : int(headerStart)+int(end)]
			if len(story) > 0 && story[len(story)-1] == 13 {
				story = story[:len(story)-1]
			}
			layout.Stories = append(layout.Stories, docx.LegacyImportStory{Kind: roles[i], Type: types[i], Text: string(utf16.Decode(story))})
		}
	}
	return layout, nil
}

func (n nativeWord) part(index int) ([]byte, error) {
	if index*8+8 > len(n.fib) {
		return nil, errors.New("missing Word FIB offset")
	}
	p := n.fib[index*8:]
	return binarySpan(n.table, binary.LittleEndian.Uint32(p), binary.LittleEndian.Uint32(p[4:]))
}

func (n nativeWord) text() ([]uint16, []uint32, error) {
	clx, err := n.part(33)
	if err != nil {
		return nil, nil, err
	}
	at := 0
	for at < len(clx) && clx[at] == 1 {
		l, err := wordU16(clx, at+1)
		if err != nil {
			return nil, nil, err
		}
		at += 3 + int(l)
	}
	if at+5 > len(clx) || clx[at] != 2 {
		return nil, nil, errors.New("invalid Word CLX piece table")
	}
	pieces, err := binarySpan(clx, uint32(at+5), binary.LittleEndian.Uint32(clx[at+1:]))
	if err != nil {
		return nil, nil, err
	}
	if len(pieces) < 4 || (len(pieces)-4)%12 != 0 {
		return nil, nil, errors.New("invalid Word piece PLC")
	}
	count := (len(pieces) - 4) / 12
	total := binary.LittleEndian.Uint32(pieces[count*4:])
	if uint64(total) > uint64(len(n.word)) {
		return nil, nil, errors.New("Word piece text exceeds supported backing stream")
	}
	chars := make([]uint16, total)
	positions := make([]uint32, total)
	for i := 0; i < count; i++ {
		begin := binary.LittleEndian.Uint32(pieces[4*i:])
		end := binary.LittleEndian.Uint32(pieces[4*(i+1):])
		if end < begin || end > total {
			return nil, nil, errors.New("unordered Word piece CPs")
		}
		fc := binary.LittleEndian.Uint32(pieces[4*(count+1)+8*i+2:])
		compressed := fc&0x40000000 != 0
		fc &= 0x3fffffff
		unit := uint32(2)
		if compressed {
			unit = 1
			fc /= 2
		}
		data, err := binarySpan(n.word, fc, (end-begin)*unit)
		if err != nil {
			return nil, nil, err
		}
		for j := begin; j < end; j++ {
			offset := (j - begin) * unit
			positions[j] = fc + offset
			if compressed {
				chars[j] = uint16(charmap.Windows1252.DecodeByte(data[offset]))
			} else {
				chars[j] = binary.LittleEndian.Uint16(data[offset:])
			}
		}
	}
	return chars, positions, nil
}

// SPRM operand sizes are encoded in the upper three bits of the opcode.
func readWordPRLs(data []byte, visit func(uint16, []byte)) error {
	for at := 0; at < len(data); {
		if len(data)-at == 1 && data[at] == 0 {
			return nil
		}
		code, err := wordU16(data, at)
		if err != nil {
			return err
		}
		at += 2
		if code == 0 {
			continue
		}
		size := []int{1, 1, 2, 4, 2, 2, -1, 3}[code>>13]
		if size < 0 {
			if at >= len(data) {
				return errors.New("truncated Word variable SPRM")
			}
			size = int(data[at])
			at++
		}
		if at+size > len(data) {
			return fmt.Errorf("truncated Word SPRM 0x%04x", code)
		}
		visit(code, data[at:at+size])
		at += size
	}
	return nil
}

func (n nativeWord) paragraphProperties(fc uint32) (document.WordParagraphLayout, error) {
	var out document.WordParagraphLayout
	plc, err := n.part(13)
	if err != nil {
		return out, err
	}
	if len(plc) == 0 {
		return out, nil
	}
	if len(plc) < 4 || (len(plc)-4)%8 != 0 {
		return out, errors.New("invalid Word PAPX PLC")
	}
	count := (len(plc) - 4) / 8
	for i := 0; i < count; i++ {
		start := binary.LittleEndian.Uint32(plc[4*i:])
		end := binary.LittleEndian.Uint32(plc[4*(i+1):])
		if fc < start || fc >= end {
			continue
		}
		page := binary.LittleEndian.Uint32(plc[4*(count+1)+4*i:]) & 0x3fffff
		fkp, err := binarySpan(n.word, page*512, 512)
		if err != nil {
			return out, err
		}
		runs := int(fkp[511])
		if 4*(runs+1)+runs*13 > 511 {
			return out, errors.New("invalid Word PAPX FKP")
		}
		for j := 0; j < runs; j++ {
			start := binary.LittleEndian.Uint32(fkp[4*j:])
			end := binary.LittleEndian.Uint32(fkp[4*(j+1):])
			if fc < start || fc >= end {
				continue
			}
			at := int(fkp[4*(runs+1)+13*j]) * 2
			if at == 0 {
				return out, nil
			}
			if at >= 511 {
				return out, errors.New("invalid Word PAPX offset")
			}
			size := int(fkp[at])*2 - 1
			at++
			if size < 0 {
				if at >= 511 {
					return out, errors.New("truncated Word PAPX")
				}
				size = int(fkp[at]) * 2
				at++
			}
			if size < 2 || at+size > 511 {
				return out, errors.New("truncated Word paragraph properties")
			}
			return out, readWordPRLs(fkp[at+2:at+size], func(code uint16, v []byte) {
				switch code {
				case 0x6412:
					line := int16(binary.LittleEndian.Uint16(v))
					multiple := binary.LittleEndian.Uint16(v[2:])
					out.Set |= document.WordParagraphLineSpacingSet
					if multiple == 1 {
						out.LineSpacingRule = "auto"
						out.LineSpacingMultiple = float64(line) / 240
					} else if line < 0 {
						out.LineSpacingRule = "exact"
						out.LineSpacingPT = -float64(line) / 20
					} else {
						out.LineSpacingRule = "atleast"
						out.LineSpacingPT = float64(line) / 20
					}
				case 0xa413:
					out.BeforePT = float64(binary.LittleEndian.Uint16(v)) / 20
					out.Set |= document.WordParagraphBeforeSet
				case 0xa414:
					out.AfterPT = float64(binary.LittleEndian.Uint16(v)) / 20
					out.Set |= document.WordParagraphAfterSet
				case 0x2407:
					out.PageBreakBefore = v[0] != 0
					out.Set |= document.WordParagraphPageBreakBeforeSet
				case 0x2447:
					out.SnapToGrid = v[0] != 0
					out.Set |= document.WordParagraphSnapToGridSet
				}
			})
		}
	}
	return out, nil
}
