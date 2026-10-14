package xls

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"unicode/utf16"
)

type boundsheet struct {
	Filepos uint32
	Type    byte
	Visible byte
	Name    byte
}

// WorkSheet in one WorkBook
type WorkSheet struct {
	bs   *boundsheet
	wb   *WorkBook
	Name string
	rows map[uint16]*Row
	//NOTICE: this is the max row number of the sheet, so it should be count -1
	MaxRow               uint16
	parsed               bool
	formulaGroups        map[uint32]formulaGroup
	pendingStringFormula *FormulaCol
	parseErr             error
}

func (w *WorkSheet) Row(i int) *Row {
	row := w.rows[uint16(i)]
	row.wb = w.wb
	return row
}

func (w *WorkSheet) parse(buf io.ReadSeeker) {
	w.rows = make(map[uint16]*Row)
	w.formulaGroups = make(map[uint32]formulaGroup)
	w.pendingStringFormula = nil
	w.parseErr = nil
	b := new(bof)
	var bof_pre *bof
	for {
		if err := binary.Read(buf, binary.LittleEndian, b); err != nil {
			if err != io.EOF {
				w.parseErr = err
			}
			break
		}
		payload := make([]byte, b.Size)
		if _, err := io.ReadFull(buf, payload); err != nil {
			w.parseErr = fmt.Errorf("read BIFF record 0x%04X payload: %w", b.Id, err)
			break
		}
		if w.pendingStringFormula != nil && b.Id != 0x0207 {
			w.parseErr = fmt.Errorf("string formula result is not followed by a STRING record")
			break
		}
		bof_pre = w.parseBof(bytes.NewReader(payload), b, bof_pre)
		if w.parseErr != nil {
			break
		}
		if b.Id == 0xa {
			break
		}
	}
	if w.parseErr == nil && w.pendingStringFormula != nil {
		w.parseErr = fmt.Errorf("string formula result is missing its following STRING record")
	}
	w.parsed = true
}

func (w *WorkSheet) parseBof(buf io.ReadSeeker, b *bof, pre *bof) *bof {
	var col interface{}
	switch b.Id {
	// case 0x0E5: //MERGEDCELLS
	// ws.mergedCells(buf)
	case 0x208: //ROW
		r := new(rowInfo)
		binary.Read(buf, binary.LittleEndian, r)
		w.addRow(r)
	case 0x0BD: //MULRK
		mc := new(MulrkCol)
		size := (b.Size - 6) / 6
		binary.Read(buf, binary.LittleEndian, &mc.Col)
		mc.Xfrks = make([]XfRk, size)
		for i := uint16(0); i < size; i++ {
			binary.Read(buf, binary.LittleEndian, &mc.Xfrks[i])
		}
		binary.Read(buf, binary.LittleEndian, &mc.LastColB)
		col = mc
	case 0x0BE: //MULBLANK
		mc := new(MulBlankCol)
		size := (b.Size - 6) / 2
		binary.Read(buf, binary.LittleEndian, &mc.Col)
		mc.Xfs = make([]uint16, size)
		for i := uint16(0); i < size; i++ {
			binary.Read(buf, binary.LittleEndian, &mc.Xfs[i])
		}
		binary.Read(buf, binary.LittleEndian, &mc.LastColB)
		col = mc
	case 0x203: //NUMBER
		col = new(NumberCol)
		binary.Read(buf, binary.LittleEndian, col)
	case 0x06: //FORMULA
		c := new(FormulaCol)
		if err := binary.Read(buf, binary.LittleEndian, &c.Header); err != nil {
			w.parseErr = err
			return b
		}
		c.Bts = make([]byte, int(b.Size)-20)
		if err := binary.Read(buf, binary.LittleEndian, &c.Bts); err != nil {
			w.parseErr = err
			return b
		}
		if c.Header.Result[6] == 0xFF && c.Header.Result[7] == 0xFF && c.Header.Result[0] == 0 {
			w.pendingStringFormula = c
		}
		col = c
	case 0x0207: //STRING cached result for the immediately preceding FORMULA.
		if w.pendingStringFormula == nil {
			w.parseErr = fmt.Errorf("STRING record has no preceding formula string result")
			return b
		}
		var count uint16
		if err := binary.Read(buf, binary.LittleEndian, &count); err != nil {
			w.parseErr = fmt.Errorf("read formula STRING character count: %w", err)
			return b
		}
		value, err := w.wb.get_string(buf, count)
		if err != nil {
			w.parseErr = fmt.Errorf("read formula STRING cached value: %w", err)
			return b
		}
		w.pendingStringFormula.CachedString = value
		w.pendingStringFormula.HasCachedString = true
		w.pendingStringFormula = nil
	case 0x04BC: //SHRFMLA
		record := make([]byte, int(b.Size))
		if _, err := io.ReadFull(buf, record); err != nil {
			w.parseErr = err
			return b
		}
		if err := w.addFormulaGroup(record, formulaGroupShared); err != nil {
			w.parseErr = err
		}
	case 0x0221: //ARRAY
		record := make([]byte, int(b.Size))
		if _, err := io.ReadFull(buf, record); err != nil {
			w.parseErr = err
			return b
		}
		if err := w.addFormulaGroup(record, formulaGroupArray); err != nil {
			w.parseErr = err
		}
	case 0x27e: //RK
		col = new(RkCol)
		binary.Read(buf, binary.LittleEndian, col)
	case 0xFD: //LABELSST
		col = new(LabelsstCol)
		binary.Read(buf, binary.LittleEndian, col)
	case 0x204:
		c := new(labelCol)
		binary.Read(buf, binary.LittleEndian, &c.BlankCol)
		var count uint16
		binary.Read(buf, binary.LittleEndian, &count)
		text, stringErr := w.wb.get_string(buf, count)
		if stringErr != nil {
			w.parseErr = fmt.Errorf("LABEL string: %w", stringErr)
			return b
		}
		c.Str = text
		col = c
	case 0x205: //BOOLERR
		cell := new(BoolErrCol)
		binary.Read(buf, binary.LittleEndian, &cell.Col)
		binary.Read(buf, binary.LittleEndian, &cell.Xf)
		binary.Read(buf, binary.LittleEndian, &cell.Value)
		binary.Read(buf, binary.LittleEndian, &cell.IsError)
		col = cell
	case 0x201: //BLANK
		col = new(BlankCol)
		binary.Read(buf, binary.LittleEndian, col)
	case 0x1b8: //HYPERLINK
		var hy HyperLink
		binary.Read(buf, binary.LittleEndian, &hy.CellRange)
		buf.Seek(20, 1)
		var flag uint32
		binary.Read(buf, binary.LittleEndian, &flag)
		var count uint32

		if flag&0x14 != 0 {
			binary.Read(buf, binary.LittleEndian, &count)
			hy.Description = b.utf16String(buf, count)
		}
		if flag&0x80 != 0 {
			binary.Read(buf, binary.LittleEndian, &count)
			hy.TargetFrame = b.utf16String(buf, count)
		}
		if flag&0x1 != 0 {
			var guid [2]uint64
			binary.Read(buf, binary.BigEndian, &guid)
			if guid[0] == 0xE0C9EA79F9BACE11 && guid[1] == 0x8C8200AA004BA90B { //URL
				hy.IsUrl = true
				binary.Read(buf, binary.LittleEndian, &count)
				hy.Url = b.utf16String(buf, count/2)
			} else if guid[0] == 0x303000000000000 && guid[1] == 0xC000000000000046 { //URL{
				var upCount uint16
				binary.Read(buf, binary.LittleEndian, &upCount)
				binary.Read(buf, binary.LittleEndian, &count)
				bts := make([]byte, count)
				binary.Read(buf, binary.LittleEndian, &bts)
				hy.ShortedFilePath = string(bts)
				buf.Seek(24, 1)
				binary.Read(buf, binary.LittleEndian, &count)
				if count > 0 {
					binary.Read(buf, binary.LittleEndian, &count)
					buf.Seek(2, 1)
					hy.ExtendedFilePath = b.utf16String(buf, count/2+1)
				}
			}
		}
		if flag&0x8 != 0 {
			binary.Read(buf, binary.LittleEndian, &count)
			var bts = make([]uint16, count)
			binary.Read(buf, binary.LittleEndian, &bts)
			runes := utf16.Decode(bts[:len(bts)-1])
			hy.TextMark = string(runes)
		}

		w.addRange(&hy.CellRange, &hy)
	case 0x809:
		buf.Seek(int64(b.Size), 1)
	case 0xa:
	default:
		// log.Printf("Unknow %X,%d\n", b.Id, b.Size)
		buf.Seek(int64(b.Size), 1)
	}
	if col != nil {
		w.add(col)
	}
	return b
}

func (w *WorkSheet) add(content interface{}) {
	if ch, ok := content.(contentHandler); ok {
		if col, ok := content.(Coler); ok {
			w.addCell(col, ch)
		}
	}

}

func (w *WorkSheet) addCell(col Coler, ch contentHandler) {
	w.addContent(col.Row(), ch)
}

func (w *WorkSheet) addRange(rang Ranger, ch contentHandler) {

	for i := rang.FirstRow(); i <= rang.LastRow(); i++ {
		w.addContent(i, ch)
	}
}

func (w *WorkSheet) addContent(row_num uint16, ch contentHandler) {
	var row *Row
	var ok bool
	if row, ok = w.rows[row_num]; !ok {
		info := new(rowInfo)
		info.Index = row_num
		row = w.addRow(info)
	}
	row.cols[ch.FirstCol()] = ch
}

func (w *WorkSheet) addRow(info *rowInfo) (row *Row) {
	if info.Index > w.MaxRow {
		w.MaxRow = info.Index
	}
	var ok bool
	if row, ok = w.rows[info.Index]; ok {
		row.info = info
	} else {
		row = &Row{info: info, cols: make(map[uint16]contentHandler)}
		w.rows[info.Index] = row
	}
	return
}
