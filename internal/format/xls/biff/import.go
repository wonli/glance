package xls

import (
	"encoding/binary"
	"fmt"
	"math"
	"sort"
)

// ImportedCell exposes the cell-level BIFF data needed by a loss-aware importer.
// Column is zero-based, matching the BIFF record format.
type ImportedCell struct {
	Column        uint16
	XF            uint16
	Value         any
	FormulaToken  []byte
	FormulaKind   string
	FormulaAnchor string
	FormulaRange  string
}

// ImportedRow contains cells in ascending zero-based column order.
type ImportedRow struct {
	Number       uint16
	HeightPoints float64
	Hidden       bool
	OutlineLevel uint8
	StyleXF      uint16
	HasStyle     bool
	Cells        []ImportedCell
}

// PaletteColor is one BIFF RGBQUAD palette entry.
type PaletteColor struct {
	Red      byte
	Green    byte
	Blue     byte
	Reserved byte
}

// ErrorCell retains a BIFF error code rather than converting it to display text.
type ErrorCell struct {
	Code byte
}

type formulaGroupKind byte

const (
	formulaGroupShared formulaGroupKind = iota + 1
	formulaGroupArray
)

type formulaGroup struct {
	kind                                       formulaGroupKind
	firstRow, lastRow, firstColumn, lastColumn uint16
	tokens                                     []byte
}

func (w *WorkSheet) addFormulaGroup(record []byte, kind formulaGroupKind) error {
	fixed := 10
	if kind == formulaGroupArray {
		fixed = 14
	}
	if len(record) < fixed {
		return fmt.Errorf("formula group record is %d bytes, want at least %d", len(record), fixed)
	}
	firstRow := binary.LittleEndian.Uint16(record[0:2])
	lastRow := binary.LittleEndian.Uint16(record[2:4])
	firstColumn := uint16(record[4])
	lastColumn := uint16(record[5])
	if firstRow > lastRow || firstColumn > lastColumn {
		return fmt.Errorf("formula group has an inverted range")
	}
	cceOffset := 8
	if kind == formulaGroupArray {
		cceOffset = 12
	}
	tokenLength := int(binary.LittleEndian.Uint16(record[cceOffset : cceOffset+2]))
	if tokenLength > len(record)-cceOffset-2 {
		return fmt.Errorf("formula group token length %d exceeds record payload", tokenLength)
	}
	group := formulaGroup{
		kind:     kind,
		firstRow: firstRow, lastRow: lastRow,
		firstColumn: firstColumn, lastColumn: lastColumn,
		tokens: append([]byte(nil), record[cceOffset+2:cceOffset+2+tokenLength]...),
	}
	anchorKey := formulaCellKey(firstRow, firstColumn)
	if _, exists := w.formulaGroups[anchorKey]; exists {
		return fmt.Errorf("duplicate formula group at %s", formulaCellName(firstRow, firstColumn))
	}
	w.formulaGroups[anchorKey] = group
	return nil
}

func formulaCellKey(row, column uint16) uint32 { return uint32(row)<<16 | uint32(column) }

func formulaCellName(row, column uint16) string {
	n := int(column) + 1
	var letters []byte
	for n > 0 {
		n--
		letters = append([]byte{byte('A' + n%26)}, letters...)
		n /= 26
	}
	return fmt.Sprintf("%s%d", string(letters), int(row)+1)
}

func formulaRangeName(group formulaGroup) string {
	first := formulaCellName(group.firstRow, group.firstColumn)
	last := formulaCellName(group.lastRow, group.lastColumn)
	if first == last {
		return first
	}
	return first + ":" + last
}

// ExternalSheetRef maps a BIFF EXTERNSHEET index to a SUPBOOK and sheet span.
type ExternalSheetRef struct {
	SupBook uint16
	First   uint16
	Last    uint16
}

// SheetName resolves an EXTERNSHEET index for an internal workbook reference.
func (w *WorkBook) SheetName(externSheetIndex uint16) (string, error) {
	if int(externSheetIndex) >= len(w.ExternalSheetRefs) {
		return "", fmt.Errorf("external sheet index %d is outside EXTERNSHEET table", externSheetIndex)
	}
	ref := w.ExternalSheetRefs[externSheetIndex]
	if int(ref.SupBook) >= len(w.SupBookInternal) || !w.SupBookInternal[ref.SupBook] {
		return "", fmt.Errorf("external workbook reference %d is not supported", ref.SupBook)
	}
	if ref.First > ref.Last || int(ref.Last) >= w.NumSheets() {
		return "", fmt.Errorf("external sheet span %d:%d is invalid", ref.First, ref.Last)
	}
	first := w.SheetAt(int(ref.First)).Name
	if ref.First == ref.Last {
		return first, nil
	}
	last := w.SheetAt(int(ref.Last)).Name
	return first + ":" + last, nil
}

// SheetAt returns a sheet in workbook order and parses it lazily.
func (w *WorkBook) SheetAt(index int) *WorkSheet {
	return w.GetSheet(index)
}

// XF returns the raw exported BIFF XF struct at index. Callers can type-switch
// on *Xf8 or *Xf5 without depending on the workbook's private XF interface.
func (w *WorkBook) XF(index uint16) (any, error) {
	if int(index) >= len(w.Xfs) {
		return nil, fmt.Errorf("XF index %d is outside the workbook table", index)
	}
	return w.Xfs[index], nil
}

// ColorRGB resolves a BIFF indexed color. Custom palette entries override the
// default colors at indexes 8 through 63, as defined by MS-XLS.
func (w *WorkBook) ColorRGB(index uint16) (PaletteColor, error) {
	if index <= 7 {
		colors := [...]PaletteColor{
			{Red: 0, Green: 0, Blue: 0}, {Red: 255, Green: 255, Blue: 255},
			{Red: 255, Green: 0, Blue: 0}, {Red: 0, Green: 255, Blue: 0},
			{Red: 0, Green: 0, Blue: 255}, {Red: 255, Green: 255, Blue: 0},
			{Red: 255, Green: 0, Blue: 255}, {Red: 0, Green: 255, Blue: 255},
		}
		return colors[index], nil
	}
	if index >= 8 && index <= 63 {
		paletteIndex := int(index - 8)
		if paletteIndex < len(w.Palette) {
			return w.Palette[paletteIndex], nil
		}
		return defaultPaletteColor(index), nil
	}
	switch index {
	case 0x7FFF, 0x0040:
		return PaletteColor{Red: 0, Green: 0, Blue: 0}, nil
	case 0x0041:
		return PaletteColor{Red: 255, Green: 255, Blue: 255}, nil
	default:
		return PaletteColor{}, fmt.Errorf("BIFF color index 0x%04X cannot be represented", index)
	}
}

func defaultPaletteColor(index uint16) PaletteColor {
	colors := [...]PaletteColor{
		{0, 0, 0, 0}, {255, 255, 255, 0}, {255, 0, 0, 0}, {0, 255, 0, 0},
		{0, 0, 255, 0}, {255, 255, 0, 0}, {255, 0, 255, 0}, {0, 255, 255, 0},
		{128, 0, 0, 0}, {0, 128, 0, 0}, {0, 0, 128, 0}, {128, 128, 0, 0},
		{128, 0, 128, 0}, {0, 128, 128, 0}, {192, 192, 192, 0}, {128, 128, 128, 0},
		{153, 153, 255, 0}, {153, 51, 102, 0}, {255, 255, 204, 0}, {204, 255, 255, 0},
		{102, 0, 102, 0}, {255, 128, 128, 0}, {0, 102, 204, 0}, {204, 204, 255, 0},
		{0, 0, 128, 0}, {255, 0, 255, 0}, {255, 255, 0, 0}, {0, 255, 255, 0},
		{128, 0, 128, 0}, {128, 0, 0, 0}, {0, 128, 128, 0}, {0, 0, 255, 0},
		{0, 204, 255, 0}, {204, 255, 255, 0}, {204, 255, 204, 0}, {255, 255, 153, 0},
		{153, 204, 255, 0}, {255, 153, 204, 0}, {204, 153, 255, 0}, {255, 204, 153, 0},
		{51, 102, 255, 0}, {51, 204, 204, 0}, {153, 204, 0, 0}, {255, 204, 0, 0},
		{255, 153, 0, 0}, {255, 102, 0, 0}, {102, 102, 153, 0}, {150, 150, 150, 0},
		{0, 51, 102, 0}, {51, 153, 102, 0}, {0, 51, 0, 0}, {51, 51, 0, 0},
		{153, 51, 0, 0}, {153, 51, 102, 0}, {51, 51, 153, 0}, {51, 51, 51, 0},
	}
	paletteIndex := int(index - 8)
	if paletteIndex < len(colors) {
		return colors[paletteIndex]
	}
	return PaletteColor{Red: 0, Green: 0, Blue: 0}
}

// FormatString returns a custom BIFF number format, if present.
func (w *WorkBook) FormatString(index uint16) string {
	if format := w.Formats[index]; format != nil {
		return format.str
	}
	return ""
}

// VisitImportRows visits populated BIFF rows in ascending order. It preserves
// blank cells with XF records and returns numeric values without applying a
// display format (dates are numeric cells plus number formats in BIFF).
func (w *WorkSheet) VisitImportRows(visit func(ImportedRow) error) error {
	if !w.parsed {
		w.wb.prepareSheet(w)
	}
	if w.parseErr != nil {
		return fmt.Errorf("worksheet %q: %w", w.Name, w.parseErr)
	}
	rowNumbers := make([]int, 0, len(w.rows))
	for rowNumber, row := range w.rows {
		if len(row.cols) != 0 || (row.info != nil && (row.info.Height >= 2 || row.info.Flags&0x000000E7 != 0)) {
			rowNumbers = append(rowNumbers, int(rowNumber))
		}
	}
	sort.Ints(rowNumbers)
	for _, rowNumber := range rowNumbers {
		row := w.rows[uint16(rowNumber)]
		columns := make([]int, 0, len(row.cols))
		for column := range row.cols {
			columns = append(columns, int(column))
		}
		sort.Ints(columns)
		imported := ImportedRow{Number: uint16(rowNumber)}
		if row.info != nil && row.info.Height >= 2 {
			imported.HeightPoints = float64(row.info.Height) / 20
		}
		if row.info != nil {
			imported.Hidden = row.info.Flags&0x00000020 != 0
			imported.OutlineLevel = uint8(row.info.Flags & 0x7)
			imported.HasStyle = row.info.Flags&0x00000080 != 0
			imported.StyleXF = uint16((row.info.Flags >> 16) & 0x0FFF)
		}
		for _, column := range columns {
			if err := w.appendImportCells(&imported, row.cols[uint16(column)]); err != nil {
				return fmt.Errorf("sheet %q row %d column %d: %w", w.Name, rowNumber+1, column+1, err)
			}
		}
		sort.Slice(imported.Cells, func(i, j int) bool { return imported.Cells[i].Column < imported.Cells[j].Column })
		if err := visit(imported); err != nil {
			return err
		}
	}
	return nil
}

func (w *WorkSheet) appendImportCells(row *ImportedRow, source any) error {
	appendCell := func(column, xf uint16, value any, tokens []byte) {
		row.Cells = append(row.Cells, ImportedCell{
			Column:       column,
			XF:           xf,
			Value:        value,
			FormulaToken: tokens,
		})
	}
	switch cell := source.(type) {
	case *NumberCol:
		appendCell(cell.FirstCol(), cell.Index, cell.Float, nil)
	case *RkCol:
		appendCell(cell.FirstCol(), cell.Xfrk.Index, rkNumber(cell.Xfrk.Rk), nil)
	case *MulrkCol:
		for offset, value := range cell.Xfrks {
			appendCell(cell.FirstCol()+uint16(offset), value.Index, rkNumber(value.Rk), nil)
		}
	case *LabelsstCol:
		if uint64(cell.Sst) >= uint64(len(w.wb.sst)) {
			return fmt.Errorf("shared-string index %d is out of range", cell.Sst)
		}
		appendCell(cell.FirstCol(), cell.Xf, w.wb.sst[cell.Sst], nil)
	case *labelCol:
		appendCell(cell.FirstCol(), cell.Xf, cell.Str, nil)
	case *BoolErrCol:
		if cell.IsError != 0 {
			appendCell(cell.FirstCol(), cell.Xf, ErrorCell{Code: cell.Value}, nil)
		} else {
			appendCell(cell.FirstCol(), cell.Xf, cell.Value != 0, nil)
		}
	case *BlankCol:
		appendCell(cell.FirstCol(), cell.Xf, nil, nil)
	case *MulBlankCol:
		for offset, xf := range cell.Xfs {
			appendCell(cell.FirstCol()+uint16(offset), xf, nil, nil)
		}
	case *FormulaCol:
		tokens, err := cell.formulaTokens()
		if err != nil {
			return err
		}
		formulaKind, formulaAnchor, formulaRange := "", "", ""
		if len(tokens) == 5 && tokens[0] == 0x01 {
			anchorRow := binary.LittleEndian.Uint16(tokens[1:3])
			anchorColumn := binary.LittleEndian.Uint16(tokens[3:5])
			group, ok := w.formulaGroups[formulaCellKey(anchorRow, anchorColumn)]
			if !ok {
				return fmt.Errorf("PtgExp points to missing shared/array formula anchor %s", formulaCellName(anchorRow, anchorColumn))
			}
			if uint16(row.Number) < group.firstRow || uint16(row.Number) > group.lastRow || cell.Header.FirstCol() < group.firstColumn || cell.Header.FirstCol() > group.lastColumn {
				return fmt.Errorf("formula cell is outside its shared/array range %s", formulaRangeName(group))
			}
			tokens = append([]byte(nil), group.tokens...)
			formulaAnchor = formulaCellName(anchorRow, anchorColumn)
			formulaRange = formulaRangeName(group)
			if group.kind == formulaGroupShared {
				formulaKind = "shared"
			} else {
				formulaKind = "array"
			}
		}
		value, err := cell.cachedValue()
		if err != nil {
			return err
		}
		// Some producers cache a bare PtgBool as numeric 0/1 instead of using
		// BIFF's special boolean formula-result marker. The token is unambiguous
		// when it is the complete expression, so restore its source value type.
		if len(tokens) == 2 && tokens[0] == 0x1D {
			value = tokens[1] != 0
		}
		row.Cells = append(row.Cells, ImportedCell{
			Column: cell.Header.FirstCol(), XF: cell.Header.IndexXf,
			Value: value, FormulaToken: tokens, FormulaKind: formulaKind,
			FormulaAnchor: formulaAnchor, FormulaRange: formulaRange,
		})
	}
	return nil
}

func rkNumber(value RK) float64 {
	raw := uint32(value)
	var number float64
	if raw&2 != 0 {
		number = float64(int32(raw) >> 2)
	} else {
		number = math.Float64frombits(uint64(raw&0xFFFFFFFC) << 32)
	}
	if raw&1 != 0 {
		number /= 100
	}
	return number
}

func (c *FormulaCol) Row() uint16      { return c.Header.Row() }
func (c *FormulaCol) FirstCol() uint16 { return c.Header.FirstCol() }
func (c *FormulaCol) LastCol() uint16  { return c.Header.LastCol() }

func (c *FormulaCol) formulaTokens() ([]byte, error) {
	if len(c.Bts) < 2 {
		return nil, fmt.Errorf("formula token length is missing")
	}
	tokenLength := int(binary.LittleEndian.Uint16(c.Bts[:2]))
	if tokenLength > len(c.Bts)-2 {
		return nil, fmt.Errorf("formula token length %d exceeds record payload %d", tokenLength, len(c.Bts)-2)
	}
	return append([]byte(nil), c.Bts[2:2+tokenLength]...), nil
}

func (c *FormulaCol) cachedValue() (any, error) {
	result := c.Header.Result
	if result[6] == 0xFF && result[7] == 0xFF {
		switch result[0] {
		case 0:
			if !c.HasCachedString {
				return nil, fmt.Errorf("cached string formula result has no following STRING record")
			}
			return c.CachedString, nil
		case 1:
			return result[2] != 0, nil
		case 2:
			return ErrorCell{Code: result[2]}, nil
		case 3:
			return nil, nil
		default:
			return nil, fmt.Errorf("unknown cached formula result type %d", result[0])
		}
	}
	return math.Float64frombits(binary.LittleEndian.Uint64(result[:])), nil
}

// BoolErrCol represents BIFF BOOLERR cell records.
type BoolErrCol struct {
	Col
	Xf      uint16
	Value   byte
	IsError byte
}
