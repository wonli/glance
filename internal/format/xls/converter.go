package xls

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/xuri/excelize/v2"
)

const defaultXLSImportCellBudget uint64 = 100_000

// TempXLSX owns an isolated on-disk conversion result. Its directory is
// removed after the XLSX adapter opens the workbook or when import fails.
type TempXLSX struct {
	Path      string
	directory string
	mu        sync.Mutex
	cleaned   bool
}

// ConvertToTempXLSX stages legacy BIFF data in an app-owned temporary XLSX.
// The normal XLSX adapter opens the staged workbook as the active document.
func ConvertToTempXLSX(ctx context.Context, sourcePath string) (*TempXLSX, error) {
	return convertToTempXLSX(ctx, sourcePath, defaultXLSImportCellBudget)
}

func convertToTempXLSX(ctx context.Context, sourcePath string, maxCells uint64) (*TempXLSX, error) {
	if ctx == nil {
		return nil, errors.New("XLS import context is nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if maxCells == 0 {
		return nil, errors.New("XLS import cell budget must be positive")
	}
	reader, err := openImportReader(sourcePath, maxCells)
	if err != nil {
		return nil, err
	}
	converted, convertErr := convertImportReader(ctx, reader, maxCells)
	closeErr := reader.Close()
	if closeErr != nil {
		if converted != nil {
			_ = converted.Cleanup()
		}
		if convertErr != nil {
			return nil, errors.Join(convertErr, fmt.Errorf("close XLS import reader: %w", closeErr))
		}
		return nil, fmt.Errorf("close XLS import reader: %w", closeErr)
	}
	return converted, convertErr
}

// Cleanup removes the private conversion directory and is safe to call twice.
func (t *TempXLSX) Cleanup() error {
	if t == nil || t.directory == "" {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cleaned {
		return nil
	}
	if err := os.RemoveAll(t.directory); err != nil {
		return err
	}
	t.cleaned = true
	return nil
}

func convertImportReader(ctx context.Context, reader ImportReader, maxCells uint64) (_ *TempXLSX, err error) {
	if reader == nil {
		return nil, errors.New("XLS import reader is nil")
	}
	if ctx == nil {
		return nil, errors.New("XLS import context is nil")
	}
	if maxCells == 0 {
		return nil, errors.New("XLS import cell budget must be positive")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for _, feature := range reader.UnsupportedFeatures() {
		location := ""
		if feature.Sheet != "" {
			location = fmt.Sprintf(" in worksheet %q", feature.Sheet)
		}
		return nil, fmt.Errorf("XLS feature %q%s (BIFF record 0x%04X) cannot be preserved by the XLSX importer", feature.Name, location, feature.RecordID)
	}
	sheetNames := reader.SheetNames()
	if len(sheetNames) == 0 {
		return nil, errors.New("XLS workbook has no worksheets")
	}

	directory, err := os.MkdirTemp("", "glance-xls-import-*")
	if err != nil {
		return nil, fmt.Errorf("create private XLS import directory: %w", err)
	}
	keepDirectory := false
	defer func() {
		if !keepDirectory {
			if removeErr := os.RemoveAll(directory); removeErr != nil {
				err = errors.Join(err, fmt.Errorf("remove failed XLS import directory: %w", removeErr))
			}
		}
	}()
	outputPath := filepath.Join(directory, "converted.xlsx")
	workbook := excelize.NewFile()
	workbookClosed := false
	defer func() {
		if !workbookClosed {
			if closeErr := workbook.Close(); closeErr != nil {
				err = errors.Join(err, fmt.Errorf("close staged XLSX workbook: %w", closeErr))
			}
		}
	}()

	defaultSheet := workbook.GetSheetList()[0]
	if err := workbook.SetSheetName(defaultSheet, sheetNames[0]); err != nil {
		return nil, fmt.Errorf("create XLSX worksheet %q: %w", sheetNames[0], err)
	}
	for _, sheet := range sheetNames[1:] {
		if _, err := workbook.NewSheet(sheet); err != nil {
			return nil, fmt.Errorf("create XLSX worksheet %q: %w", sheet, err)
		}
	}
	visibleSheets := 0
	for _, sheet := range sheetNames {
		hidden, _ := reader.SheetVisibility(sheet)
		if !hidden {
			visibleSheets++
		}
	}
	if visibleSheets == 0 {
		return nil, errors.New("XLS workbook has no visible worksheet")
	}
	for _, sheet := range sheetNames {
		hidden, veryHidden := reader.SheetVisibility(sheet)
		if hidden {
			if err := workbook.SetSheetVisible(sheet, false, veryHidden); err != nil {
				return nil, fmt.Errorf("preserve visibility of worksheet %q: %w", sheet, err)
			}
		}
	}
	resolver, ok := reader.(FormulaResolver)
	if !ok {
		return nil, errors.New("XLS import reader does not provide BIFF formula references")
	}

	patches := make(map[string]worksheetPatches)
	styleIDs := make(map[uint16]int)
	styleForXF := func(xf uint16) (int, error) {
		if styleID, found := styleIDs[xf]; found {
			return styleID, nil
		}
		style, err := reader.ExcelizeStyle(xf)
		if err != nil {
			return 0, err
		}
		styleID, err := workbook.NewStyle(&style)
		if err != nil {
			return 0, err
		}
		styleIDs[xf] = styleID
		return styleID, nil
	}
	var importedCells uint64
	for _, sheetName := range sheetNames {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		stream, err := workbook.NewStreamWriter(sheetName)
		if err != nil {
			return nil, fmt.Errorf("create XLSX row writer for worksheet %q: %w", sheetName, err)
		}
		for _, width := range reader.ColumnWidths(sheetName) {
			if err := stream.SetColWidth(width.FirstColumn, width.LastColumn, width.WidthChars); err != nil {
				return nil, fmt.Errorf("worksheet %q columns %d:%d: preserve BIFF width: %w", sheetName, width.FirstColumn, width.LastColumn, err)
			}
			styleID, err := styleForXF(width.StyleXF)
			if err != nil {
				return nil, fmt.Errorf("worksheet %q columns %d:%d: map BIFF XF %d: %w", sheetName, width.FirstColumn, width.LastColumn, width.StyleXF, err)
			}
			if err := stream.SetColStyle(width.FirstColumn, width.LastColumn, styleID); err != nil {
				return nil, fmt.Errorf("worksheet %q columns %d:%d: preserve BIFF XF %d: %w", sheetName, width.FirstColumn, width.LastColumn, width.StyleXF, err)
			}
			if width.Hidden {
				if err := stream.SetColVisible(width.FirstColumn, width.LastColumn, false); err != nil {
					return nil, fmt.Errorf("worksheet %q columns %d:%d: preserve hidden state: %w", sheetName, width.FirstColumn, width.LastColumn, err)
				}
			}
			if width.OutlineLevel > 0 {
				for column := width.FirstColumn; column <= width.LastColumn; column++ {
					if err := stream.SetColOutlineLevel(column, width.OutlineLevel); err != nil {
						return nil, fmt.Errorf("worksheet %q column %d: preserve outline level %d: %w", sheetName, column, width.OutlineLevel, err)
					}
				}
			}
			if width.BestFit || width.Collapsed {
				patch := patches[sheetName]
				patch.columnProperties = append(patch.columnProperties, width)
				patches[sheetName] = patch
			}
		}
		for _, cellRange := range reader.MergedCells(sheetName) {
			if err := stream.MergeCell(cellRange[0], cellRange[1]); err != nil {
				return nil, fmt.Errorf("worksheet %q: preserve merged range %s:%s: %w", sheetName, cellRange[0], cellRange[1], err)
			}
		}
		previousRow := 0
		visitErr := reader.VisitRows(sheetName, func(row ImportedRow) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if row.Number < 1 || row.Number > 65536 {
				return fmt.Errorf("worksheet %q has BIFF row %d outside BIFF8 limits", sheetName, row.Number)
			}
			if row.Number <= previousRow {
				return fmt.Errorf("worksheet %q rows are not strictly ascending: %d after %d", sheetName, row.Number, previousRow)
			}
			previousRow = row.Number
			values := make([]interface{}, 0)
			previousColumn := 0
			for _, cell := range row.Cells {
				if cell.Column < 1 || cell.Column > 256 {
					return fmt.Errorf("worksheet %q row %d has BIFF column %d outside BIFF8 limits", sheetName, row.Number, cell.Column)
				}
				if cell.Column <= previousColumn {
					return fmt.Errorf("worksheet %q row %d columns are not strictly ascending: %d after %d", sheetName, row.Number, cell.Column, previousColumn)
				}
				previousColumn = cell.Column
				if importedCells == maxCells {
					return fmt.Errorf("XLS import exceeds the configured %d-cell resource budget; no cells were truncated", maxCells)
				}
				importedCells++
				for len(values) < cell.Column {
					values = append(values, nil)
				}
				styleID, err := styleForXF(cell.XF)
				if err != nil {
					return fmt.Errorf("worksheet %q cell %s: map XF %d: %w", sheetName, cellName(cell.Column, row.Number), cell.XF, err)
				}
				ref := cellName(cell.Column, row.Number)
				formula := ""
				if len(cell.FormulaToken) != 0 {
					formula, err = importedFormula(cell, ref, resolver)
					if err != nil {
						return fmt.Errorf("worksheet %q cell %s: decode formula: %w", sheetName, ref, err)
					}
					if cell.FormulaKind == "array" {
						if cell.FormulaAnchor == "" || cell.FormulaRange == "" {
							return fmt.Errorf("worksheet %q cell %s: array formula has no anchor/range metadata", sheetName, ref)
						}
						if ref != cell.FormulaAnchor {
							formula = ""
						} else {
							patch := patches[sheetName]
							if patch.arrayFormulas == nil {
								patch.arrayFormulas = make(map[string]string)
							}
							patch.arrayFormulas[ref] = cell.FormulaRange
							patches[sheetName] = patch
						}
					}
				}
				value, isError, err := importedXLSXValue(cell.Value)
				if err != nil {
					return fmt.Errorf("worksheet %q cell %s: %w", sheetName, ref, err)
				}
				if isError {
					patch := patches[sheetName]
					if patch.errorCells == nil {
						patch.errorCells = make(map[string]struct{})
					}
					patch.errorCells[ref] = struct{}{}
					patches[sheetName] = patch
				}
				values[cell.Column-1] = excelize.Cell{StyleID: styleID, Formula: formula, Value: value}
			}
			if len(values) == 0 && row.HeightPoints <= 0 && !row.Hidden && row.OutlineLevel == 0 && !row.HasStyle {
				return nil
			}
			start, err := excelize.CoordinatesToCellName(1, row.Number)
			if err != nil {
				return err
			}
			rowStyleID := 0
			if row.HasStyle {
				rowStyleID, err = styleForXF(row.StyleXF)
				if err != nil {
					return fmt.Errorf("worksheet %q row %d: map BIFF row XF %d: %w", sheetName, row.Number, row.StyleXF, err)
				}
			}
			if err := stream.SetRow(start, values, excelize.RowOpts{
				Height: row.HeightPoints, Hidden: row.Hidden,
				OutlineLevel: int(row.OutlineLevel), StyleID: rowStyleID,
			}); err != nil {
				return fmt.Errorf("worksheet %q row %d: write streamed XLSX row: %w", sheetName, row.Number, err)
			}
			return nil
		})
		if visitErr != nil {
			return nil, visitErr
		}
		if err := stream.Flush(); err != nil {
			return nil, fmt.Errorf("worksheet %q: flush streamed XLSX rows: %w", sheetName, err)
		}
	}
	for _, sheetName := range sheetNames {
		for _, comment := range reader.Comments(sheetName) {
			if err := workbook.AddComment(sheetName, excelize.Comment{Cell: comment.Cell, Author: comment.Author, Text: comment.Text}); err != nil {
				return nil, fmt.Errorf("worksheet %q cell %s: preserve legacy comment: %w", sheetName, comment.Cell, err)
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := workbook.SaveAs(outputPath); err != nil {
		return nil, fmt.Errorf("write staged XLSX workbook: %w", err)
	}
	if err := workbook.Close(); err != nil {
		workbookClosed = true
		return nil, fmt.Errorf("close staged XLSX workbook: %w", err)
	}
	workbookClosed = true
	if len(patches) != 0 {
		if err := patchXLSXPackage(outputPath, patches); err != nil {
			return nil, fmt.Errorf("preserve imported BIFF metadata in staged XLSX: %w", err)
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	keepDirectory = true
	return &TempXLSX{Path: outputPath, directory: directory}, nil
}

func importedFormula(cell ImportedCell, cellRef string, resolver FormulaResolver) (string, error) {
	if cell.FormulaKind == "shared" {
		if cell.FormulaAnchor == "" {
			return "", errors.New("shared formula has no anchor metadata")
		}
		return decodeBIFFSharedFormula(cell.FormulaToken, cell.FormulaAnchor, cellRef, resolver)
	}
	if cell.FormulaKind == "array" {
		if cell.FormulaAnchor == "" || cell.FormulaRange == "" {
			return "", errors.New("array formula has no anchor/range metadata")
		}
		return decodeBIFFArrayFormula(cell.FormulaToken, cell.FormulaAnchor, cell.FormulaRange, resolver)
	}
	return decodeBIFFFormula(cell.FormulaToken, cellRef, resolver)
}

func importedXLSXValue(value any) (any, bool, error) {
	if cellError, ok := value.(CellError); ok {
		errorValue := xlsErrorString(cellError.Code)
		if strings.HasPrefix(errorValue, "#XLSERR_") {
			return nil, false, fmt.Errorf("BIFF error code 0x%02X cannot be represented in XLSX", cellError.Code)
		}
		return errorValue, true, nil
	}
	return value, false, nil
}

func xlsErrorString(code byte) string {
	switch code {
	case 0x00:
		return "#NULL!"
	case 0x07:
		return "#DIV/0!"
	case 0x0F:
		return "#VALUE!"
	case 0x17:
		return "#REF!"
	case 0x1D:
		return "#NAME?"
	case 0x24:
		return "#NUM!"
	case 0x2A:
		return "#N/A"
	case 0x2B:
		return "#GETTING_DATA"
	default:
		return fmt.Sprintf("#XLSERR_%02X", code)
	}
}

func cellName(column, row int) string {
	name, _ := excelize.CoordinatesToCellName(column, row)
	return name
}

type worksheetPatches struct {
	errorCells       map[string]struct{}
	arrayFormulas    map[string]string
	columnProperties []ImportedColumnWidth
}

type packageWorkbook struct {
	Sheets []struct {
		Name string `xml:"name,attr"`
		ID   string `xml:"id,attr"`
	} `xml:"sheets>sheet"`
}

type packageRelationships struct {
	Relationships []struct {
		ID     string `xml:"Id,attr"`
		Target string `xml:"Target,attr"`
	} `xml:"Relationship"`
}

func patchXLSXPackage(filename string, patches map[string]worksheetPatches) (err error) {
	archive, err := zip.OpenReader(filename)
	if err != nil {
		return err
	}
	defer archive.Close()
	parts, err := xlsxWorksheetParts(&archive.Reader)
	if err != nil {
		return err
	}
	partPatches := make(map[string]worksheetPatches, len(patches))
	for sheet, patch := range patches {
		part, ok := parts[sheet]
		if !ok {
			return fmt.Errorf("worksheet %q has no package relationship", sheet)
		}
		partPatches[path.Clean(part)] = patch
	}
	tempFile, err := os.CreateTemp(filepath.Dir(filename), ".xls-package-patch-*.xlsx")
	if err != nil {
		return err
	}
	tempPath := tempFile.Name()
	fileClosed := false
	defer func() {
		if !fileClosed {
			if closeErr := tempFile.Close(); closeErr != nil {
				err = errors.Join(err, closeErr)
			}
		}
		if err != nil {
			_ = os.Remove(tempPath)
		}
	}()
	writer := zip.NewWriter(tempFile)
	for _, entry := range archive.File {
		header := entry.FileHeader
		out, createErr := writer.CreateHeader(&header)
		if createErr != nil {
			_ = writer.Close()
			return fmt.Errorf("copy XLSX package entry %q: %w", entry.Name, createErr)
		}
		input, openErr := entry.Open()
		if openErr != nil {
			_ = writer.Close()
			return fmt.Errorf("open XLSX package entry %q: %w", entry.Name, openErr)
		}
		patch, shouldPatch := partPatches[path.Clean(entry.Name)]
		var copyErr error
		if shouldPatch {
			copyErr = rewriteWorksheetXML(out, input, patch)
		} else {
			_, copyErr = io.Copy(out, input)
		}
		closeErr := input.Close()
		if copyErr != nil {
			_ = writer.Close()
			if closeErr != nil {
				return errors.Join(fmt.Errorf("rewrite XLSX package entry %q: %w", entry.Name, copyErr), closeErr)
			}
			return fmt.Errorf("rewrite XLSX package entry %q: %w", entry.Name, copyErr)
		}
		if closeErr != nil {
			_ = writer.Close()
			return fmt.Errorf("close XLSX package entry %q: %w", entry.Name, closeErr)
		}
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("finish patched XLSX package: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		fileClosed = true
		return err
	}
	fileClosed = true
	if err := os.Rename(tempPath, filename); err != nil {
		return fmt.Errorf("replace staged XLSX package with fidelity patches: %w", err)
	}
	return nil
}

func xlsxWorksheetParts(archive *zip.Reader) (map[string]string, error) {
	workbookData, err := readZIPEntry(archive, "xl/workbook.xml")
	if err != nil {
		return nil, err
	}
	relationshipData, err := readZIPEntry(archive, "xl/_rels/workbook.xml.rels")
	if err != nil {
		return nil, err
	}
	var workbook packageWorkbook
	if err := xml.Unmarshal(workbookData, &workbook); err != nil {
		return nil, fmt.Errorf("parse XLSX workbook part: %w", err)
	}
	var relationships packageRelationships
	if err := xml.Unmarshal(relationshipData, &relationships); err != nil {
		return nil, fmt.Errorf("parse XLSX workbook relationships: %w", err)
	}
	targets := make(map[string]string, len(relationships.Relationships))
	for _, relationship := range relationships.Relationships {
		if relationship.ID != "" {
			targets[relationship.ID] = relationship.Target
		}
	}
	result := make(map[string]string, len(workbook.Sheets))
	for _, sheet := range workbook.Sheets {
		target, ok := targets[sheet.ID]
		if !ok {
			return nil, fmt.Errorf("XLSX worksheet %q refers to missing relationship %q", sheet.Name, sheet.ID)
		}
		target = strings.TrimPrefix(target, "/")
		if !strings.HasPrefix(target, "xl/") {
			target = path.Join("xl", target)
		}
		result[sheet.Name] = path.Clean(target)
	}
	return result, nil
}

func readZIPEntry(archive *zip.Reader, name string) ([]byte, error) {
	for _, entry := range archive.File {
		if entry.Name != name {
			continue
		}
		reader, err := entry.Open()
		if err != nil {
			return nil, err
		}
		data, readErr := io.ReadAll(reader)
		return data, errors.Join(readErr, reader.Close())
	}
	return nil, fmt.Errorf("XLSX package is missing %q", name)
}

func rewriteWorksheetXML(dst io.Writer, src io.Reader, patch worksheetPatches) error {
	decoder := xml.NewDecoder(src)
	encoder := xml.NewEncoder(dst)
	var currentCell string
	var inCell bool
	var skipColumnEnd bool
	matchedErrors := make(map[string]struct{}, len(patch.errorCells))
	matchedArrays := make(map[string]struct{}, len(patch.arrayFormulas))
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		switch value := token.(type) {
		case xml.StartElement:
			if value.Name.Local == "col" && len(patch.columnProperties) > 0 {
				if err := encodePatchedColumns(encoder, value, patch.columnProperties); err != nil {
					return err
				}
				skipColumnEnd = true
				continue
			}
			if value.Name.Local == "c" {
				inCell = true
				currentCell = xmlAttribute(value.Attr, "r")
				if _, ok := patch.errorCells[currentCell]; ok {
					value.Attr = setXMLAttribute(value.Attr, "t", "e")
					matchedErrors[currentCell] = struct{}{}
				}
			}
			if inCell && value.Name.Local == "f" {
				if formulaRange, ok := patch.arrayFormulas[currentCell]; ok {
					value.Attr = setXMLAttribute(value.Attr, "t", "array")
					value.Attr = setXMLAttribute(value.Attr, "ref", formulaRange)
					matchedArrays[currentCell] = struct{}{}
				}
			}
			if err := encoder.EncodeToken(value); err != nil {
				return err
			}
		case xml.EndElement:
			if value.Name.Local == "col" && skipColumnEnd {
				skipColumnEnd = false
				continue
			}
			if err := encoder.EncodeToken(value); err != nil {
				return err
			}
			if value.Name.Local == "c" {
				inCell = false
				currentCell = ""
			}
		default:
			if err := encoder.EncodeToken(token); err != nil {
				return err
			}
		}
	}
	if err := encoder.Flush(); err != nil {
		return err
	}
	if len(matchedErrors) != len(patch.errorCells) {
		return fmt.Errorf("matched %d of %d cached error cells", len(matchedErrors), len(patch.errorCells))
	}
	if len(matchedArrays) != len(patch.arrayFormulas) {
		return fmt.Errorf("matched %d of %d array formula anchors", len(matchedArrays), len(patch.arrayFormulas))
	}
	return nil
}

func encodePatchedColumns(encoder *xml.Encoder, original xml.StartElement, properties []ImportedColumnWidth) error {
	minimum, err := strconv.Atoi(xmlAttribute(original.Attr, "min"))
	if err != nil {
		return fmt.Errorf("parse XLSX column min attribute: %w", err)
	}
	maximum, err := strconv.Atoi(xmlAttribute(original.Attr, "max"))
	if err != nil || minimum < 1 || maximum < minimum {
		return fmt.Errorf("invalid XLSX column range %d:%d", minimum, maximum)
	}
	boundaries := []int{minimum, maximum + 1}
	for _, property := range properties {
		if property.FirstColumn > maximum || property.LastColumn < minimum {
			continue
		}
		boundaries = append(boundaries, max(minimum, property.FirstColumn), min(maximum+1, property.LastColumn+1))
	}
	sort.Ints(boundaries)
	unique := boundaries[:0]
	for _, boundary := range boundaries {
		if len(unique) == 0 || unique[len(unique)-1] != boundary {
			unique = append(unique, boundary)
		}
	}
	for index := 0; index+1 < len(unique); index++ {
		first, last := unique[index], unique[index+1]-1
		attributes := append([]xml.Attr(nil), original.Attr...)
		attributes = setXMLAttribute(attributes, "min", strconv.Itoa(first))
		attributes = setXMLAttribute(attributes, "max", strconv.Itoa(last))
		for _, property := range properties {
			if property.FirstColumn <= first && property.LastColumn >= last {
				if property.BestFit {
					attributes = setXMLAttribute(attributes, "bestFit", "true")
				}
				if property.Collapsed {
					attributes = setXMLAttribute(attributes, "collapsed", "true")
				}
			}
		}
		start := xml.StartElement{Name: original.Name, Attr: attributes}
		if err := encoder.EncodeToken(start); err != nil {
			return err
		}
		if err := encoder.EncodeToken(start.End()); err != nil {
			return err
		}
	}
	return nil
}

func xmlAttribute(attributes []xml.Attr, name string) string {
	for _, attribute := range attributes {
		if attribute.Name.Local == name {
			return attribute.Value
		}
	}
	return ""
}

func setXMLAttribute(attributes []xml.Attr, name, value string) []xml.Attr {
	for index := range attributes {
		if attributes[index].Name.Local == name {
			attributes[index].Value = value
			return attributes
		}
	}
	return append(attributes, xml.Attr{Name: xml.Name{Local: name}, Value: value})
}
