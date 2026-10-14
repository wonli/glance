package xls

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"unicode/utf16"
)

// xls workbook type
type WorkBook struct {
	Is5ver            bool
	Type              uint16
	Codepage          uint16
	Xfs               []st_xf_data
	Fonts             []Font
	Formats           map[uint16]*Format
	Palette           []PaletteColor
	SupBookInternal   []bool
	ExternalSheetRefs []ExternalSheetRef
	//All the sheets from the workbook
	sheets         []*WorkSheet
	Author         string
	rs             io.ReadSeeker
	sst            []string
	continue_utf16 uint16
	continue_rich  uint16
	continue_apsb  uint32
	parseErr       error
	dateMode       uint16
}

// read workbook from ole2 file
func newWorkBookFromOle2(rs io.ReadSeeker) *WorkBook {
	wb := new(WorkBook)
	wb.Formats = make(map[uint16]*Format)
	// wb.bts = bts
	wb.rs = rs
	wb.sheets = make([]*WorkSheet, 0)
	wb.Parse(rs)
	return wb
}

func (w *WorkBook) Parse(buf io.ReadSeeker) {
	var previous, pending *bof
	for {
		record := pending
		pending = nil
		if record == nil {
			record = new(bof)
			if err := binary.Read(buf, binary.LittleEndian, record); err != nil {
				if err != io.EOF && err != io.ErrUnexpectedEOF {
					w.parseErr = fmt.Errorf("read BIFF record header: %w", err)
				}
				return
			}
		}
		if record.Id == 0x00FC {
			payload := make([]byte, record.Size)
			if _, err := io.ReadFull(buf, payload); err != nil {
				w.parseErr = fmt.Errorf("read SST record: %w", err)
				return
			}
			segments := [][]byte{payload}
			var totalBytes uint64 = uint64(len(payload))
			for {
				next := new(bof)
				if err := binary.Read(buf, binary.LittleEndian, next); err != nil {
					if err != io.EOF && err != io.ErrUnexpectedEOF {
						w.parseErr = fmt.Errorf("read BIFF SST continuation header: %w", err)
						return
					}
					break
				}
				if next.Id != 0x003C {
					pending = next
					break
				}
				continuation := make([]byte, next.Size)
				if _, err := io.ReadFull(buf, continuation); err != nil {
					w.parseErr = fmt.Errorf("read BIFF SST continuation: %w", err)
					return
				}
				totalBytes += uint64(len(continuation))
				if totalBytes > maxSSTBytes {
					w.parseErr = fmt.Errorf("SST data exceeds the %d MiB parser budget", maxSSTBytes>>20)
					return
				}
				segments = append(segments, continuation)
			}
			if w.parseErr = w.parseSST(segments); w.parseErr != nil {
				return
			}
			previous = record
			continue
		}
		previous, _, _ = w.parseBof(buf, record, previous, 0)
		if w.parseErr != nil {
			return
		}
	}
}

// ParseError reports a fatal malformed shared-string record detected while parsing.
func (w *WorkBook) ParseError() error { return w.parseErr }

func (w *WorkBook) addXf(xf st_xf_data) {
	w.Xfs = append(w.Xfs, xf)
}

func (w *WorkBook) addFont(font *FontInfo, buf io.ReadSeeker) {
	name, _ := w.get_string(buf, uint16(font.NameB))
	w.Fonts = append(w.Fonts, Font{Info: font, Name: name})
}

func (w *WorkBook) addFormat(format *Format) {
	if w.Formats == nil {
		os.Exit(1)
	}
	w.Formats[format.Head.Index] = format
}

func (wb *WorkBook) parseBof(buf io.ReadSeeker, b *bof, pre *bof, offset_pre int) (after *bof, after_using *bof, offset int) {
	after = b
	after_using = pre
	var bts = make([]byte, b.Size)
	binary.Read(buf, binary.LittleEndian, bts)
	buf_item := bytes.NewReader(bts)
	switch b.Id {
	case 0x809:
		bif := new(biffHeader)
		binary.Read(buf_item, binary.LittleEndian, bif)
		if bif.Ver != 0x600 {
			wb.Is5ver = true
		}
		wb.Type = bif.Type
	case 0x042: // CODEPAGE
		binary.Read(buf_item, binary.LittleEndian, &wb.Codepage)
	case 0x3c: // CONTINUE (SST continuations are consumed in Parse)
		buf.Seek(int64(b.Size), io.SeekCurrent)
	case 0xfc: // SST (parsed together with CONTINUE records in Parse)
		buf.Seek(int64(b.Size), io.SeekCurrent)
	case 0x85: // bOUNDSHEET
		var bs = new(boundsheet)
		binary.Read(buf_item, binary.LittleEndian, bs)
		// different for BIFF5 and BIFF8
		wb.addSheet(bs, buf_item)
	case 0x0e0: // XF
		if wb.Is5ver {
			xf := new(Xf5)
			binary.Read(buf_item, binary.LittleEndian, xf)
			wb.addXf(xf)
		} else {
			xf := new(Xf8)
			binary.Read(buf_item, binary.LittleEndian, xf)
			wb.addXf(xf)
		}
	case 0x031: // FONT
		f := new(FontInfo)
		binary.Read(buf_item, binary.LittleEndian, f)
		wb.addFont(f, buf_item)
	case 0x41E: //FORMAT
		font := new(Format)
		binary.Read(buf_item, binary.LittleEndian, &font.Head)
		font.str, _ = wb.get_string(buf_item, font.Head.Size)
		wb.addFormat(font)
	case 0x22: //DATEMODE
		binary.Read(buf_item, binary.LittleEndian, &wb.dateMode)
	case 0x92: //PALETTE
		var count uint16
		if binary.Read(buf_item, binary.LittleEndian, &count) == nil && count <= 56 {
			colors := make([]PaletteColor, count)
			if binary.Read(buf_item, binary.LittleEndian, &colors) == nil {
				wb.Palette = colors
			}
		}
	case 0x01AE: //SUPBOOK
		var sheetCount, marker uint16
		if binary.Read(buf_item, binary.LittleEndian, &sheetCount) != nil || binary.Read(buf_item, binary.LittleEndian, &marker) != nil {
			wb.SupBookInternal = append(wb.SupBookInternal, false)
		} else {
			wb.SupBookInternal = append(wb.SupBookInternal, marker == 0x0401)
		}
	case 0x0017: //EXTERNSHEET
		var count uint16
		if binary.Read(buf_item, binary.LittleEndian, &count) == nil && int(count) <= buf_item.Len()/6 {
			for i := uint16(0); i < count; i++ {
				var ref ExternalSheetRef
				if binary.Read(buf_item, binary.LittleEndian, &ref) != nil {
					break
				}
				wb.ExternalSheetRefs = append(wb.ExternalSheetRefs, ref)
			}
		}
	}
	return
}

func (w *WorkBook) get_string(buf io.ReadSeeker, size uint16) (res string, err error) {
	if w.Is5ver {
		var bts = make([]byte, size)
		_, err = buf.Read(bts)
		res = string(bts)
	} else {
		var richtext_num = uint16(0)
		var phonetic_size = uint32(0)
		var flag byte
		err = binary.Read(buf, binary.LittleEndian, &flag)
		if flag&0x8 != 0 {
			err = binary.Read(buf, binary.LittleEndian, &richtext_num)
		} else if w.continue_rich > 0 {
			richtext_num = w.continue_rich
			w.continue_rich = 0
		}
		if flag&0x4 != 0 {
			err = binary.Read(buf, binary.LittleEndian, &phonetic_size)
		} else if w.continue_apsb > 0 {
			phonetic_size = w.continue_apsb
			w.continue_apsb = 0
		}
		if flag&0x1 != 0 {
			var bts = make([]uint16, size)
			var i = uint16(0)
			for ; i < size && err == nil; i++ {
				err = binary.Read(buf, binary.LittleEndian, &bts[i])
			}
			runes := utf16.Decode(bts[:i])
			res = string(runes)
			if i < size {
				w.continue_utf16 = size - i
			}
		} else {
			var bts = make([]byte, size)
			var n int
			n, err = buf.Read(bts)
			if uint16(n) < size {
				w.continue_utf16 = size - uint16(n)
				err = io.EOF
			}

			var bts1 = make([]uint16, size)
			for k, v := range bts[:n] {
				bts1[k] = uint16(v)
			}
			runes := utf16.Decode(bts1)
			res = string(runes)
		}
		if richtext_num > 0 {
			var bts []byte
			var seek_size int64
			if w.Is5ver {
				seek_size = int64(2 * richtext_num)
			} else {
				seek_size = int64(4 * richtext_num)
			}
			bts = make([]byte, seek_size)
			err = binary.Read(buf, binary.LittleEndian, bts)
			if err == io.EOF {
				w.continue_rich = richtext_num
			}

			// err = binary.Read(buf, binary.LittleEndian, bts)
		}
		if phonetic_size > 0 {
			if phonetic_size > 8224 {
				return "", fmt.Errorf("phonetic extension length %d exceeds the 8,224-byte BIFF record limit (string length %d, flags 0x%02X)", phonetic_size, size, flag)
			}
			var bts []byte
			bts = make([]byte, phonetic_size)
			err = binary.Read(buf, binary.LittleEndian, bts)
			if err == io.EOF {
				w.continue_apsb = phonetic_size
			}
		}
	}
	return
}

func (w *WorkBook) addSheet(sheet *boundsheet, buf io.ReadSeeker) {
	name, _ := w.get_string(buf, uint16(sheet.Name))
	w.sheets = append(w.sheets, &WorkSheet{bs: sheet, Name: name, wb: w})
}

// reading a sheet from the compress file to memory, you should call this before you try to get anything from sheet
func (w *WorkBook) prepareSheet(sheet *WorkSheet) {
	w.rs.Seek(int64(sheet.bs.Filepos), 0)
	sheet.parse(w.rs)
}

// Get one sheet by its number
func (w *WorkBook) GetSheet(num int) *WorkSheet {
	if num < len(w.sheets) {
		s := w.sheets[num]
		if !s.parsed {
			w.prepareSheet(s)
		}
		return s
	} else {
		return nil
	}
}

// Get the number of all sheets, look into example
func (w *WorkBook) NumSheets() int {
	return len(w.sheets)
}

// helper function to read all cells from file
// Notice: the max value is the limit of the max capacity of lines.
// Warning: the helper function will need big memeory if file is large.
func (w *WorkBook) ReadAllCells(max int) (res [][]string) {
	res = make([][]string, 0)
	for _, sheet := range w.sheets {
		if len(res) < max {
			max = max - len(res)
			w.prepareSheet(sheet)
			if sheet.MaxRow != 0 {
				leng := int(sheet.MaxRow) + 1
				if max < leng {
					leng = max
				}
				temp := make([][]string, leng)
				for k, row := range sheet.rows {
					data := make([]string, 0)
					if len(row.cols) > 0 {
						for _, col := range row.cols {
							if uint16(len(data)) <= col.LastCol() {
								data = append(data, make([]string, col.LastCol()-uint16(len(data))+1)...)
							}
							str := col.String(w)

							for i := uint16(0); i < col.LastCol()-col.FirstCol()+1; i++ {
								data[col.FirstCol()+i] = str[i]
							}
						}
						if leng > int(k) {
							temp[k] = data
						}
					}
				}
				res = append(res, temp...)
			}
		}
	}
	return
}
