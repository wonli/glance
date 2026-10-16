package xlsx

import (
	"bytes"
	"encoding/xml"
	"glance/internal/document"
	"io"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func scopedPrefixedWorkbook(t *testing.T) *Workbook {
	t.Helper()
	w := NewBlank()
	t.Cleanup(func() { w.Close() })
	if err := w.file.SetCellValue("Sheet1", "A100", "keep"); err != nil {
		t.Fatal(err)
	}
	before, err := w.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	files, target, err := scopePackage(before, "Sheet1")
	if err != nil {
		t.Fatal(err)
	}
	raw := string(files[target])
	raw = strings.Replace(raw, `xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"`, `xmlns:s="http://schemas.openxmlformats.org/spreadsheetml/2006/main"`, 1)
	raw = regexp.MustCompile(`<(/?)([A-Za-z][A-Za-z0-9]*)([ >])`).ReplaceAllString(raw, `<${1}s:${2}${3}`)
	raw = strings.ReplaceAll(raw, "><", ">\n<")
	if _, err := parseScopeXML([]byte(raw)); err != nil {
		t.Fatalf("invalid prefixed fixture: %v\n%s", err, raw)
	}
	files[target] = []byte(raw)
	updated, err := replaceScopePackage(before, files)
	if err != nil {
		t.Fatal(err)
	}
	if err = w.restoreSnapshotLocked(updated); err != nil {
		t.Fatal(err)
	}
	return w
}

func TestScopedFormattingRetainsWorksheetNamespace(t *testing.T) {
	for _, bounds := range []document.CellRange{
		{StartRow: 2, EndRow: 2, StartColumn: 1, EndColumn: maxXLSXColumns},
		{StartRow: 1, EndRow: maxXLSXRows, StartColumn: 2, EndColumn: 2},
	} {
		t.Run(CellName(bounds.StartColumn, bounds.StartRow), func(t *testing.T) {
			w := scopedPrefixedWorkbook(t)
			family := "Namespace Test Font"
			if _, err := w.SetCellFormatsChanged("Sheet1", bounds, document.CellFormatPatch{FontFamily: &family}); err != nil {
				t.Fatal(err)
			}
			snapshot, err := w.Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			files, target, err := scopePackage(snapshot, "Sheet1")
			if err != nil {
				t.Fatal(err)
			}
			d := xml.NewDecoder(bytes.NewReader(files[target]))
			lastRow := 0
			for {
				token, err := d.Token()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				start, ok := token.(xml.StartElement)
				if !ok {
					continue
				}
				switch start.Name.Local {
				case "row", "c", "cols", "col", "sheetData":
					if start.Name.Space != "http://schemas.openxmlformats.org/spreadsheetml/2006/main" {
						t.Fatalf("%s belongs to namespace %q; inserted worksheet nodes must retain namespace", start.Name.Local, start.Name.Space)
					}
				}
				if start.Name.Local == "row" {
					var row int
					for _, attr := range start.Attr {
						if attr.Name.Local == "r" {
							row, _ = strconv.Atoi(attr.Value)
						}
					}
					if row < lastRow {
						t.Fatalf("new sparse row %d was appended after row %d", row, lastRow)
					}
					lastRow = row
				}
			}
		})
	}
}
