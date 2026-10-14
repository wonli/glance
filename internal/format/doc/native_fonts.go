package doc

import (
	"encoding/binary"
	"errors"
	"glance/internal/document"
	"sort"
	"unicode/utf16"
)

type nativeFontRange struct {
	start, end uint32
	fonts      document.WordTextStyle
}

func (n nativeWord) fontRanges() ([]nativeFontRange, error) {
	data, err := n.part(15)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	if len(data) < 4 || binary.LittleEndian.Uint16(data[2:]) != 0 {
		return nil, errors.New("invalid Word font table")
	}
	count := int(binary.LittleEndian.Uint16(data))
	fonts := make([]string, 0, count)
	at := 4
	for i := 0; i < count; i++ {
		if at >= len(data) {
			return nil, errors.New("truncated Word font table")
		}
		size := int(data[at]) + 1
		if size < 42 || at+size > len(data) || (size-40)%2 != 0 {
			return nil, errors.New("invalid Word FFN font record")
		}
		var name []uint16
		terminated := false
		for off := at + 40; off < at+size; off += 2 {
			c := binary.LittleEndian.Uint16(data[off:])
			if c == 0 {
				terminated = true
				break
			}
			name = append(name, c)
		}
		if !terminated || len(name) == 0 {
			return nil, errors.New("invalid Word font name")
		}
		fonts = append(fonts, string(utf16.Decode(name)))
		at += size
	}
	plc, err := n.part(12)
	if err != nil {
		return nil, err
	}
	if len(plc) == 0 {
		return nil, nil
	}
	if len(plc) < 4 || (len(plc)-4)%8 != 0 {
		return nil, errors.New("invalid Word CHPX PLC")
	}
	var ranges []nativeFontRange
	runs := (len(plc) - 4) / 8
	for i := 0; i < runs; i++ {
		page := binary.LittleEndian.Uint32(plc[4*(runs+1)+4*i:]) & 0x3fffff
		fkp, err := binarySpan(n.word, page*512, 512)
		if err != nil {
			return nil, err
		}
		count := int(fkp[511])
		if count < 1 || count > 101 {
			return nil, errors.New("invalid Word CHPX FKP")
		}
		for j := 0; j < count; j++ {
			lo, hi := binary.LittleEndian.Uint32(fkp[4*j:]), binary.LittleEndian.Uint32(fkp[4*(j+1):])
			if hi <= lo {
				return nil, errors.New("unordered Word CHPX ranges")
			}
			off := int(fkp[4*(count+1)+j]) * 2
			var style document.WordTextStyle
			if off != 0 {
				if off < 4*(count+1)+count || off >= 511 {
					return nil, errors.New("invalid Word CHPX offset")
				}
				size := int(fkp[off])
				if off+1+size > 511 {
					return nil, errors.New("truncated Word character properties")
				}
				invalidFont := false
				err := readWordPRLs(fkp[off+1:off+1+size], func(code uint16, v []byte) {
					if code < 0x4a4f || code > 0x4a51 && code != 0x4a5e {
						return
					}
					index := int(binary.LittleEndian.Uint16(v))
					if index >= len(fonts) {
						invalidFont = true
						return
					}
					switch code {
					case 0x4a4f:
						style.FontFamilyASCII = fonts[index]
					case 0x4a50:
						style.FontFamilyEastAsia = fonts[index]
					case 0x4a51:
						style.FontFamilyHighANSI = fonts[index]
					case 0x4a5e:
						style.FontFamilyComplexScript = fonts[index]
					}
				})
				if err != nil {
					return nil, err
				}
				if invalidFont {
					return nil, errors.New("Word character font index outside table")
				}
			}
			ranges = append(ranges, nativeFontRange{lo, hi, style})
		}
	}
	sort.Slice(ranges, func(i, j int) bool { return ranges[i].start < ranges[j].start })
	for i := 1; i < len(ranges); i++ {
		if ranges[i].start < ranges[i-1].end {
			return nil, errors.New("overlapping Word CHPX ranges")
		}
	}
	return ranges, nil
}

// Only repair paragraphs whose entire visible text has identical direct font
// slots. Mixed/inherited font runs keep the converter's formatting until full
// character-style and piece-level property inheritance is supported.
func uniformParagraphFonts(positions []uint32, ranges []nativeFontRange) document.WordTextStyle {
	var out document.WordTextStyle
	for i, fc := range positions {
		j := sort.Search(len(ranges), func(j int) bool { return ranges[j].end > fc })
		if j == len(ranges) || fc < ranges[j].start {
			return document.WordTextStyle{}
		}
		if i == 0 {
			out = ranges[j].fonts
		} else if out != ranges[j].fonts {
			return document.WordTextStyle{}
		}
	}
	return out
}
