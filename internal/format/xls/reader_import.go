package xls

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"unicode/utf16"

	"github.com/extrame/ole2"
	legacy "github.com/extrame/xls"
	"github.com/xuri/excelize/v2"
)

// ImportedCell retains the source coordinate, value type, formula token stream,
// and BIFF XF index. Column and ImportedRow.Number are one-based.
type ImportedCell struct {
	Column        int
	Value         any
	FormulaToken  []byte
	XF            uint16
	FormulaKind   string
	FormulaAnchor string
	FormulaRange  string
}

// CellError keeps the native BIFF error code for an XLSX t="e" output cell.
type CellError struct {
	Code byte
}

// ImportedRow carries cells and row geometry in ascending source-row order.
type ImportedRow struct {
	Number       int
	HeightPoints float64
	Hidden       bool
	OutlineLevel uint8
	StyleXF      uint16
	HasStyle     bool
	Cells        []ImportedCell
}

// ImportedColumnWidth carries a BIFF column-width range. Column numbers are
// one-based and LastColumn is inclusive.
type ImportedColumnWidth struct {
	FirstColumn  int
	LastColumn   int
	WidthChars   float64
	StyleXF      uint16
	Hidden       bool
	OutlineLevel uint8
	Collapsed    bool
	BestFit      bool
}

type ImportedComment struct {
	Cell    string
	Author  string
	Text    string
	Visible bool
}

// UnsupportedFeature identifies source content that this importer cannot keep.
type UnsupportedFeature struct {
	Sheet    string
	Name     string
	RecordID uint16
}

// ImportReader exposes BIFF cell and style metadata without flattening sheets.
type ImportReader interface {
	SheetNames() []string
	SheetVisibility(sheet string) (hidden, veryHidden bool)
	VisitRows(sheet string, visit func(ImportedRow) error) error
	ColumnWidths(sheet string) []ImportedColumnWidth
	MergedCells(sheet string) [][2]string
	Comments(sheet string) []ImportedComment
	ExcelizeStyle(xf uint16) (excelize.Style, error)
	UnsupportedFeatures() []UnsupportedFeature
	Close() error
}

type importReader struct {
	file         *os.File
	book         *legacy.WorkBook
	sheetNames   []string
	features     []UnsupportedFeature
	columnWidths map[int][]ImportedColumnWidth
	sheetStates  map[int]byte
	mergedCells  map[int][][2]string
	comments     map[int][]ImportedComment
}

var _ ImportReader = (*importReader)(nil)

// OpenImportReader validates the compound document, BIFF record boundaries,
// and cell resource budget before asking the legacy reader to decode worksheet
// records. This catches encryption, malformed lengths, truncation, and
// over-budget workbooks before legacy code can materialize their rows.
func OpenImportReader(path string) (_ ImportReader, err error) {
	return openImportReader(path, defaultXLSImportCellBudget)
}

func openImportReader(path string, maxCells uint64) (_ ImportReader, err error) {
	if maxCells == 0 {
		return nil, errors.New("XLS import cell budget must be positive")
	}
	inspection, err := inspectBIFFWorkbook(path, maxCells)
	if err != nil {
		return nil, fmt.Errorf("inspect legacy XLS %q: %w", path, err)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open legacy XLS %q: %w", path, err)
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			_ = file.Close()
			err = fmt.Errorf("parse legacy XLS %q: malformed BIFF data: %v", path, recovered)
		}
	}()
	book, err := legacy.OpenReader(file, "utf-8")
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("parse legacy XLS %q: %w", path, err)
	}
	if book == nil || book.NumSheets() == 0 {
		_ = file.Close()
		return nil, errors.New("legacy XLS workbook contains no worksheets")
	}
	reader := &importReader{
		file: file, book: book, features: inspection.features,
		columnWidths: inspection.columnWidths, sheetStates: inspection.sheetStates,
		mergedCells: inspection.mergedCells, comments: inspection.comments,
	}
	for index := 0; index < book.NumSheets(); index++ {
		sheet := book.SheetAt(index)
		if sheet == nil {
			_ = file.Close()
			return nil, fmt.Errorf("legacy XLS worksheet %d is unavailable", index+1)
		}
		reader.sheetNames = append(reader.sheetNames, sheet.Name)
	}
	return reader, nil
}

func (r *importReader) SheetNames() []string {
	return append([]string(nil), r.sheetNames...)
}

func (r *importReader) VisitRows(sheetName string, visit func(ImportedRow) error) error {
	if visit == nil {
		return errors.New("row visitor is nil")
	}
	for index, name := range r.sheetNames {
		if name != sheetName {
			continue
		}
		sheet := r.book.SheetAt(index)
		return sheet.VisitImportRows(func(source legacy.ImportedRow) error {
			row := ImportedRow{
				Number: int(source.Number) + 1, HeightPoints: source.HeightPoints,
				Hidden: source.Hidden, OutlineLevel: source.OutlineLevel,
				StyleXF: source.StyleXF, HasStyle: source.HasStyle,
				Cells: make([]ImportedCell, 0, len(source.Cells)),
			}
			for _, cell := range source.Cells {
				var value any = cell.Value
				if sourceError, ok := cell.Value.(legacy.ErrorCell); ok {
					value = CellError{Code: sourceError.Code}
				}
				row.Cells = append(row.Cells, ImportedCell{
					Column:        int(cell.Column) + 1,
					Value:         value,
					FormulaToken:  append([]byte(nil), cell.FormulaToken...),
					XF:            cell.XF,
					FormulaKind:   cell.FormulaKind,
					FormulaAnchor: cell.FormulaAnchor,
					FormulaRange:  cell.FormulaRange,
				})
			}
			return visit(row)
		})
	}
	return fmt.Errorf("worksheet %q does not exist", sheetName)
}

func (r *importReader) ColumnWidths(sheetName string) []ImportedColumnWidth {
	for index, name := range r.sheetNames {
		if name == sheetName {
			return append([]ImportedColumnWidth(nil), r.columnWidths[index]...)
		}
	}
	return nil
}

func (r *importReader) MergedCells(sheetName string) [][2]string {
	for index, name := range r.sheetNames {
		if name == sheetName {
			return append([][2]string(nil), r.mergedCells[index]...)
		}
	}
	return nil
}

func (r *importReader) SheetVisibility(sheetName string) (hidden, veryHidden bool) {
	for index, name := range r.sheetNames {
		if name == sheetName {
			state := r.sheetStates[index]
			return state == 1 || state == 2, state == 2
		}
	}
	return false, false
}

func (r *importReader) Comments(sheetName string) []ImportedComment {
	for index, name := range r.sheetNames {
		if name == sheetName {
			return append([]ImportedComment(nil), r.comments[index]...)
		}
	}
	return nil
}

func (r *importReader) SheetName(externSheetIndex uint16) (string, error) {
	return r.book.SheetName(externSheetIndex)
}

func (r *importReader) DefinedName(nameIndex uint16) (string, error) {
	return "", fmt.Errorf("defined name %d is not available in the XLS name table", nameIndex)
}

var _ FormulaResolver = (*importReader)(nil)

func (r *importReader) UnsupportedFeatures() []UnsupportedFeature {
	return append([]UnsupportedFeature(nil), r.features...)
}

func (r *importReader) Close() error {
	if r.file == nil {
		return nil
	}
	err := r.file.Close()
	r.file = nil
	return err
}

type biffInspection struct {
	features     []UnsupportedFeature
	columnWidths map[int][]ImportedColumnWidth
	sheetStates  map[int]byte
	mergedCells  map[int][][2]string
	comments     map[int][]ImportedComment
}

func inspectBIFFWorkbook(path string, maxCells uint64) (biffInspection, error) {
	file, err := os.Open(path)
	if err != nil {
		return biffInspection{}, err
	}
	defer file.Close()
	document, err := ole2.Open(file, "utf-8")
	if err != nil {
		return biffInspection{}, err
	}
	directories, err := document.ListDir()
	if err != nil {
		return biffInspection{}, fmt.Errorf("read OLE directory: %w", err)
	}
	var workbookStream, root *ole2.File
	for _, entry := range directories {
		switch entry.Name() {
		case "Workbook", "Book":
			workbookStream = entry
		case "Root Entry":
			root = entry
		}
	}
	if workbookStream == nil || root == nil {
		return biffInspection{}, errors.New("OLE document has no Workbook stream")
	}
	if workbookStream.Size < 4 {
		return biffInspection{}, errors.New("Workbook stream is too short")
	}
	stream := document.OpenFile(workbookStream, root)
	features := make(map[uint16]UnsupportedFeature)
	inspection := biffInspection{
		columnWidths: make(map[int][]ImportedColumnWidth),
		sheetStates:  make(map[int]byte),
		mergedCells:  make(map[int][][2]string),
		comments:     make(map[int][]ImportedComment),
	}
	sheetIndexByOffset := make(map[int64]int)
	objectsBySheet := make(map[int]map[uint16]uint16)
	notesBySheet := make(map[int]map[uint16]struct{})
	textObjectsBySheet := make(map[int]int)
	currentSheet := -1
	sheetCount := 0
	remaining := int64(workbookStream.Size)
	var cellCount uint64
	for offset := int64(0); offset < int64(workbookStream.Size); {
		if index, ok := sheetIndexByOffset[offset]; ok {
			currentSheet = index
		}
		if remaining < 4 {
			return biffInspection{}, fmt.Errorf("truncated BIFF record header at workbook offset %d", offset)
		}
		var header [4]byte
		if _, err := io.ReadFull(stream, header[:]); err != nil {
			return biffInspection{}, fmt.Errorf("read BIFF record header at workbook offset %d: %w", offset, err)
		}
		recordID := binary.LittleEndian.Uint16(header[:2])
		payloadSize := int64(binary.LittleEndian.Uint16(header[2:]))
		offset += 4
		remaining -= 4
		if payloadSize > remaining {
			return biffInspection{}, fmt.Errorf("truncated BIFF record 0x%04X at workbook offset %d: payload length %d, remaining %d", recordID, offset-4, payloadSize, remaining)
		}
		if recordID == 0x002F {
			return biffInspection{}, errors.New("encrypted XLS workbook is not supported (BIFF FILEPASS record found)")
		}
		consumed, err := validateBIFFRecord(recordID, payloadSize, stream)
		if err != nil {
			return biffInspection{}, fmt.Errorf("invalid BIFF record 0x%04X at workbook offset %d: %w", recordID, offset-4, err)
		}
		if recordID == 0x0085 {
			prefix := make([]byte, 8)
			if _, err := io.ReadFull(stream, prefix); err != nil {
				return biffInspection{}, fmt.Errorf("read BOUNDSHEET record at workbook offset %d: %w", offset-4, err)
			}
			nameBytes := int64(prefix[6])
			if prefix[7]&0x01 != 0 {
				nameBytes *= 2
			}
			if 8+nameBytes > payloadSize {
				return biffInspection{}, fmt.Errorf("BOUNDSHEET name length %d exceeds record payload %d", nameBytes, payloadSize-8)
			}
			sheetOffset := int64(binary.LittleEndian.Uint32(prefix[:4]))
			if sheetOffset < 0 || sheetOffset >= int64(workbookStream.Size) {
				return biffInspection{}, fmt.Errorf("BOUNDSHEET offset %d is outside Workbook stream", sheetOffset)
			}
			if _, duplicate := sheetIndexByOffset[sheetOffset]; duplicate {
				return biffInspection{}, fmt.Errorf("duplicate BOUNDSHEET offset %d", sheetOffset)
			}
			sheetIndexByOffset[sheetOffset] = sheetCount
			sheetCount++
			consumed = 8
			inspection.sheetStates[sheetCount-1] = prefix[4]
			if name, unsupported := unsupportedBoundsheetMetadata(prefix[4], prefix[5]); unsupported {
				features[recordID] = UnsupportedFeature{Name: name, RecordID: recordID}
			}
		}
		if recordID == 0x0208 {
			var prefix [16]byte
			if _, err := io.ReadFull(stream, prefix[:]); err != nil {
				return biffInspection{}, fmt.Errorf("read ROW record at workbook offset %d: %w", offset-4, err)
			}
			consumed = 16
			height := binary.LittleEndian.Uint16(prefix[6:8])
			flags := binary.LittleEndian.Uint32(prefix[12:16])
			if height < 2 || height > 8192 {
				return biffInspection{}, fmt.Errorf("ROW height %d is outside BIFF8 limits", height)
			}
			if unsupportedBIFFRowFlags(flags) {
				features[recordID] = UnsupportedFeature{Name: "collapsed outlines, phonetic rows, or row border metadata", RecordID: recordID}
			}
		}
		if recordID == 0x007D {
			prefix := make([]byte, 12)
			if _, err := io.ReadFull(stream, prefix); err != nil {
				return biffInspection{}, err
			}
			if currentSheet < 0 {
				return biffInspection{}, fmt.Errorf("COLINFO record at workbook offset %d is outside a worksheet substream", offset-4)
			}
			width, unsupported, err := parseBIFFColumnInfo(prefix)
			if err != nil {
				return biffInspection{}, fmt.Errorf("invalid COLINFO at workbook offset %d: %w", offset-4, err)
			}
			if unsupported != "" {
				features[recordID] = UnsupportedFeature{Name: unsupported, RecordID: recordID}
			} else {
				inspection.columnWidths[currentSheet] = append(inspection.columnWidths[currentSheet], width)
			}
			consumed = 12
		}
		if recordID == 0x00E5 {
			if currentSheet < 0 {
				return biffInspection{}, fmt.Errorf("MERGEDCELLS record at workbook offset %d is outside a worksheet substream", offset-4)
			}
			payload := make([]byte, payloadSize)
			if _, err := io.ReadFull(stream, payload); err != nil {
				return biffInspection{}, fmt.Errorf("read MERGEDCELLS record at workbook offset %d: %w", offset-4, err)
			}
			ranges, err := parseBIFFMergedCells(payload)
			if err != nil {
				return biffInspection{}, fmt.Errorf("invalid MERGEDCELLS record at workbook offset %d: %w", offset-4, err)
			}
			for _, merged := range ranges {
				if mergeOverlaps(inspection.mergedCells[currentSheet], merged) {
					return biffInspection{}, fmt.Errorf("overlapping BIFF merged range %s:%s", merged[0], merged[1])
				}
				inspection.mergedCells[currentSheet] = append(inspection.mergedCells[currentSheet], merged)
			}
			consumed = payloadSize
		}
		if recordID == 0x005D {
			payload := make([]byte, payloadSize)
			if _, err := io.ReadFull(stream, payload); err != nil {
				return biffInspection{}, fmt.Errorf("read OBJ record at workbook offset %d: %w", offset-4, err)
			}
			objectType, objectID, err := parseBIFFObjectHeader(payload)
			if err != nil {
				return biffInspection{}, fmt.Errorf("invalid OBJ record at workbook offset %d: %w", offset-4, err)
			}
			if currentSheet < 0 || objectType != 0x0019 {
				features[recordID] = UnsupportedFeature{Name: "BIFF drawing or form-control objects cannot be preserved", RecordID: recordID}
			} else {
				if objectsBySheet[currentSheet] == nil {
					objectsBySheet[currentSheet] = make(map[uint16]uint16)
				}
				objectsBySheet[currentSheet][objectID] = objectType
			}
			consumed = payloadSize
		}
		if recordID == 0x001C {
			if currentSheet < 0 {
				return biffInspection{}, fmt.Errorf("NOTE record at workbook offset %d is outside a worksheet substream", offset-4)
			}
			payload := make([]byte, payloadSize)
			if _, err := io.ReadFull(stream, payload); err != nil {
				return biffInspection{}, fmt.Errorf("read NOTE record at workbook offset %d: %w", offset-4, err)
			}
			comment, objectID, err := parseBIFFNote(payload)
			if err != nil {
				return biffInspection{}, fmt.Errorf("invalid NOTE record at workbook offset %d: %w", offset-4, err)
			}
			if notesBySheet[currentSheet] == nil {
				notesBySheet[currentSheet] = make(map[uint16]struct{})
			}
			notesBySheet[currentSheet][objectID] = struct{}{}
			inspection.comments[currentSheet] = append(inspection.comments[currentSheet], comment)
			consumed = payloadSize
		}
		if recordID == 0x01B6 {
			if payloadSize < 6 || currentSheet < 0 {
				return biffInspection{}, fmt.Errorf("invalid TXO record at workbook offset %d", offset-4)
			}
			prefix := make([]byte, 6)
			if _, err := io.ReadFull(stream, prefix); err != nil {
				return biffInspection{}, err
			}
			textObjectsBySheet[currentSheet]++
			if binary.LittleEndian.Uint16(prefix[2:4]) != 0 || binary.LittleEndian.Uint16(prefix[4:6]) != 0 {
				features[recordID] = UnsupportedFeature{Name: "non-empty or rich-text legacy comments cannot be preserved", RecordID: recordID}
			}
			consumed = 6
		}
		if recordID == 0x0055 {
			if payloadSize != 2 || currentSheet < 0 {
				return biffInspection{}, fmt.Errorf("invalid DEFCOLWIDTH record at workbook offset %d", offset-4)
			}
			prefix := make([]byte, 2)
			if _, err := io.ReadFull(stream, prefix); err != nil {
				return biffInspection{}, err
			}
			width := binary.LittleEndian.Uint16(prefix)
			if width > 255 {
				return biffInspection{}, fmt.Errorf("DEFCOLWIDTH value %d is outside XLSX limits", width)
			}
			if width == 0 {
				features[recordID] = UnsupportedFeature{Name: "zero default column width cannot be preserved", RecordID: recordID}
			} else {
				inspection.columnWidths[currentSheet] = append(inspection.columnWidths[currentSheet], ImportedColumnWidth{
					FirstColumn: 1,
					LastColumn:  256,
					WidthChars:  float64(width),
				})
			}
			consumed = payloadSize
		}
		cellCountForRecord, err := biffSerializedCellCount(recordID, payloadSize)
		if err != nil {
			return biffInspection{}, fmt.Errorf("invalid BIFF cell record 0x%04X at workbook offset %d: %w", recordID, offset-4, err)
		}
		if cellCountForRecord > maxCells-cellCount {
			return biffInspection{}, fmt.Errorf("XLS import exceeds the configured %d-cell resource budget before BIFF worksheet parsing", maxCells)
		}
		cellCount += cellCountForRecord
		if name, unsupported := unsupportedBIFFRecord(recordID); unsupported {
			features[recordID] = UnsupportedFeature{Name: name, RecordID: recordID}
		}
		if _, err := io.CopyN(io.Discard, stream, payloadSize-consumed); err != nil {
			return biffInspection{}, fmt.Errorf("read BIFF record 0x%04X payload at workbook offset %d: %w", recordID, offset-4, err)
		}
		offset += payloadSize
		remaining -= payloadSize
	}
	for sheet, objects := range objectsBySheet {
		for objectID := range objects {
			if _, ok := notesBySheet[sheet][objectID]; !ok {
				features[0x005D] = UnsupportedFeature{Name: "unmatched BIFF note object cannot be preserved", RecordID: 0x005D}
			}
		}
	}
	for sheet := range notesBySheet {
		if textObjectsBySheet[sheet] < len(notesBySheet[sheet]) {
			features[0x01B6] = UnsupportedFeature{Name: "legacy comment text records are incomplete", RecordID: 0x01B6}
		}
	}
	inspection.features = make([]UnsupportedFeature, 0, len(features))
	for _, feature := range features {
		inspection.features = append(inspection.features, feature)
	}
	sort.Slice(inspection.features, func(i, j int) bool { return inspection.features[i].RecordID < inspection.features[j].RecordID })
	return inspection, nil
}

func parseBIFFMergedCells(payload []byte) ([][2]string, error) {
	if len(payload) < 2 {
		return nil, fmt.Errorf("payload length %d is smaller than the count field", len(payload))
	}
	count := int(binary.LittleEndian.Uint16(payload[:2]))
	if count > 1026 || len(payload) != 2+count*8 {
		return nil, fmt.Errorf("merged range count %d does not match payload length %d", count, len(payload))
	}
	ranges := make([][2]string, 0, count)
	for i := 0; i < count; i++ {
		offset := 2 + i*8
		firstRow := int(binary.LittleEndian.Uint16(payload[offset : offset+2]))
		lastRow := int(binary.LittleEndian.Uint16(payload[offset+2 : offset+4]))
		firstColumn := int(binary.LittleEndian.Uint16(payload[offset+4 : offset+6]))
		lastColumn := int(binary.LittleEndian.Uint16(payload[offset+6 : offset+8]))
		if firstRow > lastRow || firstColumn > lastColumn || lastColumn > 255 {
			return nil, fmt.Errorf("merged range %d has invalid BIFF8 coordinates", i+1)
		}
		first, err := excelize.CoordinatesToCellName(firstColumn+1, firstRow+1)
		if err != nil {
			return nil, err
		}
		last, err := excelize.CoordinatesToCellName(lastColumn+1, lastRow+1)
		if err != nil {
			return nil, err
		}
		ranges = append(ranges, [2]string{first, last})
	}
	return ranges, nil
}

func mergeOverlaps(existing [][2]string, candidate [2]string) bool {
	firstCol, firstRow, err := excelize.CellNameToCoordinates(candidate[0])
	if err != nil {
		return true
	}
	lastCol, lastRow, err := excelize.CellNameToCoordinates(candidate[1])
	if err != nil {
		return true
	}
	for _, current := range existing {
		currentFirstCol, currentFirstRow, err := excelize.CellNameToCoordinates(current[0])
		if err != nil {
			return true
		}
		currentLastCol, currentLastRow, err := excelize.CellNameToCoordinates(current[1])
		if err != nil {
			return true
		}
		if firstCol <= currentLastCol && lastCol >= currentFirstCol && firstRow <= currentLastRow && lastRow >= currentFirstRow {
			return true
		}
	}
	return false
}

func parseBIFFObjectHeader(payload []byte) (objectType, objectID uint16, err error) {
	if len(payload) < 22 || binary.LittleEndian.Uint16(payload[:2]) != 0x0015 || binary.LittleEndian.Uint16(payload[2:4]) != 0x0012 {
		return 0, 0, fmt.Errorf("missing or invalid FtCmo object header")
	}
	return binary.LittleEndian.Uint16(payload[4:6]), binary.LittleEndian.Uint16(payload[6:8]), nil
}

func parseBIFFNote(payload []byte) (ImportedComment, uint16, error) {
	if len(payload) < 11 {
		return ImportedComment{}, 0, fmt.Errorf("payload length %d is too short", len(payload))
	}
	row := binary.LittleEndian.Uint16(payload[:2])
	column := binary.LittleEndian.Uint16(payload[2:4])
	flags := binary.LittleEndian.Uint16(payload[4:6])
	objectID := binary.LittleEndian.Uint16(payload[6:8])
	if column > 255 || objectID == 0 {
		return ImportedComment{}, 0, fmt.Errorf("cell column %d or object identifier %d is invalid", column, objectID)
	}
	author, consumed, err := parseBIFFUnicodeString(payload[8:])
	if err != nil {
		return ImportedComment{}, 0, fmt.Errorf("decode comment author: %w", err)
	}
	if len(payload)-8-consumed > 1 {
		return ImportedComment{}, 0, fmt.Errorf("comment author has %d unexpected trailing bytes", len(payload)-8-consumed)
	}
	cell, err := excelize.CoordinatesToCellName(int(column)+1, int(row)+1)
	if err != nil {
		return ImportedComment{}, 0, err
	}
	return ImportedComment{Cell: cell, Author: author, Visible: flags&0x0002 != 0}, objectID, nil
}

func parseBIFFUnicodeString(payload []byte) (string, int, error) {
	if len(payload) < 3 {
		return "", 0, fmt.Errorf("string header is truncated")
	}
	count := int(binary.LittleEndian.Uint16(payload[:2]))
	options := payload[2]
	if options&0xF2 != 0 {
		return "", 0, fmt.Errorf("unsupported Unicode string options 0x%02X", options)
	}
	offset := 3
	richRunBytes := 0
	extendedBytes := 0
	if options&0x08 != 0 {
		if len(payload)-offset < 2 {
			return "", 0, fmt.Errorf("rich-text run count is truncated")
		}
		richRunBytes = int(binary.LittleEndian.Uint16(payload[offset:offset+2])) * 4
		offset += 2
	}
	if options&0x04 != 0 {
		if len(payload)-offset < 4 {
			return "", 0, fmt.Errorf("phonetic data length is truncated")
		}
		extendedBytes = int(binary.LittleEndian.Uint32(payload[offset : offset+4]))
		offset += 4
	}
	charBytes := count
	if options&0x01 != 0 {
		charBytes *= 2
	}
	if charBytes > len(payload)-offset || richRunBytes+extendedBytes > len(payload)-offset-charBytes {
		return "", 0, fmt.Errorf("Unicode string length exceeds its record")
	}
	var value string
	if options&0x01 != 0 {
		units := make([]uint16, count)
		for i := range units {
			units[i] = binary.LittleEndian.Uint16(payload[offset+i*2 : offset+i*2+2])
		}
		value = string(utf16.Decode(units))
	} else {
		runes := make([]rune, count)
		for i := range runes {
			runes[i] = rune(payload[offset+i])
		}
		value = string(runes)
	}
	return value, offset + charBytes + richRunBytes + extendedBytes, nil
}

func parseBIFFColumnInfo(payload []byte) (ImportedColumnWidth, string, error) {
	if len(payload) != 12 {
		return ImportedColumnWidth{}, "", fmt.Errorf("payload length %d, want 12", len(payload))
	}
	first := binary.LittleEndian.Uint16(payload[0:2])
	last := binary.LittleEndian.Uint16(payload[2:4])
	width := binary.LittleEndian.Uint16(payload[4:6])
	xf := binary.LittleEndian.Uint16(payload[6:8])
	options := binary.LittleEndian.Uint16(payload[8:10])
	if first > last || last > 256 {
		return ImportedColumnWidth{}, "", fmt.Errorf("column range %d:%d is outside BIFF8 limits", first, last)
	}
	if first == 256 {
		return ImportedColumnWidth{}, "default column formatting cannot be preserved", nil
	}
	if options&0x0008 != 0 {
		return ImportedColumnWidth{}, "phonetic column metadata cannot be preserved", nil
	}
	if float64(width)/256 > 255 {
		return ImportedColumnWidth{}, "", fmt.Errorf("column width %d is outside XLSX limits", width)
	}
	if width == 0 {
		return ImportedColumnWidth{}, "zero-width columns cannot be preserved", nil
	}
	lastColumn := int(last) + 1
	if last == 256 {
		lastColumn = 256
	}
	return ImportedColumnWidth{
		FirstColumn:  int(first) + 1,
		LastColumn:   lastColumn,
		WidthChars:   float64(width) / 256,
		StyleXF:      xf,
		Hidden:       options&0x0001 != 0,
		BestFit:      options&0x0004 != 0,
		OutlineLevel: uint8((options >> 8) & 0x7),
		Collapsed:    options&0x1000 != 0,
	}, "", nil
}

func unsupportedBoundsheetMetadata(state, sheetType byte) (string, bool) {
	if sheetType != 0 {
		return "non-worksheet sheet type", true
	}
	return "", false
}

func unsupportedBIFFRowFlags(flags uint32) bool {
	const unsupported = uint32(0x00000010 | 0x30000000 | 0x40000000)
	return flags&unsupported != 0
}

func validateBIFFRecord(recordID uint16, payloadSize int64, stream io.Reader) (int64, error) {
	minimums := map[uint16]int64{
		0x0085: 8,  // BOUNDSHEET fixed header.
		0x0006: 22, // FORMULA header and token length.
		0x04BC: 10, // SHRFMLA range, use count, and token length.
		0x0221: 14, // ARRAY range, options, reserved bytes, and token length.
		0x0201: 6,  // BLANK.
		0x0203: 14, // NUMBER.
		0x0204: 8,  // LABEL header and text length.
		0x0205: 8,  // BOOLERR.
		0x0208: 16, // ROW.
		0x00BD: 12, // MULRK.
		0x00BE: 8,  // MULBLANK.
		0x00FD: 10, // LABELSST.
		0x027E: 10, // RK.
		0x0207: 3,  // STRING cached formula result.
		0x007D: 12, // COLINFO.
		0x005D: 22, // OBJ FtCmo header.
		0x001C: 11, // NOTE coordinates, object ID, and author header.
		0x00E5: 2,  // MERGEDCELLS count.
		0x01B6: 6,  // TXO header.
	}
	if minimum, known := minimums[recordID]; known && payloadSize < minimum {
		return 0, fmt.Errorf("payload length %d is smaller than required %d", payloadSize, minimum)
	}
	if recordID == 0x0092 && payloadSize != 226 {
		return 0, fmt.Errorf("PALETTE payload length %d, want 226", payloadSize)
	}
	formulaLengthOffset := int64(-1)
	if recordID == 0x04BC {
		formulaLengthOffset = 8
	}
	if recordID == 0x0221 {
		formulaLengthOffset = 12
	}
	if formulaLengthOffset >= 0 {
		if _, err := io.CopyN(io.Discard, stream, formulaLengthOffset); err != nil {
			return 0, err
		}
		var prefix [2]byte
		if _, err := io.ReadFull(stream, prefix[:]); err != nil {
			return formulaLengthOffset, err
		}
		if int64(binary.LittleEndian.Uint16(prefix[:])) > payloadSize-formulaLengthOffset-2 {
			return formulaLengthOffset + 2, fmt.Errorf("formula token length exceeds record payload")
		}
		return formulaLengthOffset + 2, nil
	}
	if recordID == 0x0006 {
		var prefix [22]byte
		if _, err := io.ReadFull(stream, prefix[:]); err != nil {
			return 0, err
		}
		tokenSize := int64(binary.LittleEndian.Uint16(prefix[20:22]))
		if tokenSize > payloadSize-22 {
			return 22, fmt.Errorf("formula token length %d exceeds payload %d", tokenSize, payloadSize-22)
		}
		return 22, nil
	}
	if recordID == 0x00BD && (payloadSize-6)%6 != 0 {
		return 0, fmt.Errorf("MULRK payload length %d is not aligned", payloadSize)
	}
	if recordID == 0x00BE && (payloadSize-6)%2 != 0 {
		return 0, fmt.Errorf("MULBLANK payload length %d is not aligned", payloadSize)
	}
	return 0, nil
}

func biffSerializedCellCount(recordID uint16, payloadSize int64) (uint64, error) {
	switch recordID {
	case 0x0006, 0x0201, 0x0203, 0x0204, 0x0205, 0x00FD, 0x027E:
		return 1, nil
	case 0x00BD:
		if payloadSize < 12 || (payloadSize-6)%6 != 0 {
			return 0, fmt.Errorf("MULRK payload length %d is invalid", payloadSize)
		}
		return uint64((payloadSize - 6) / 6), nil
	case 0x00BE:
		if payloadSize < 8 || (payloadSize-6)%2 != 0 {
			return 0, fmt.Errorf("MULBLANK payload length %d is invalid", payloadSize)
		}
		return uint64((payloadSize - 6) / 2), nil
	default:
		return 0, nil
	}
}

func unsupportedBIFFRecord(recordID uint16) (string, bool) {
	switch recordID {
	case 0x01B8:
		return "hyperlinks cannot be preserved", true
	default:
		return "", false
	}
}
