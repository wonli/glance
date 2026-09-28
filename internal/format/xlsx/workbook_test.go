package xlsx

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"glance/internal/document"

	"github.com/xuri/excelize/v2"
)

func TestWorkbookEditSaveRoundTrip(t *testing.T) {
	f := excelize.NewFile()
	if err := f.SetCellValue("Sheet1", "A1", "原始内容"); err != nil {
		t.Fatal(err)
	}
	style, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellStyle("Sheet1", "A1", "A1", style); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "roundtrip.xlsx")
	if err := f.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	wb, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer wb.Close()
	if err := wb.SetCellText("Sheet1", "A1", "修改后"); err != nil {
		t.Fatal(err)
	}
	if err := wb.Save(); err != nil {
		t.Fatal(err)
	}

	reopened, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := reopened.GetCellValue("Sheet1", "A1")
	if err != nil || got != "修改后" {
		t.Fatalf("saved value = %q, err = %v", got, err)
	}
	gotStyle, err := reopened.GetCellStyle("Sheet1", "A1")
	if err != nil || gotStyle != style {
		t.Fatalf("saved style = %d, want %d, err = %v", gotStyle, style, err)
	}
}

func TestWorkbookRejectsFormulaLikeText(t *testing.T) {
	f := excelize.NewFile()
	path := filepath.Join(t.TempDir(), "formula.xlsx")
	if err := f.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	wb, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer wb.Close()
	if err := wb.SetCellText("Sheet1", "A1", "=1+1"); err == nil {
		t.Fatal("expected formula-like input to be rejected")
	}
}

func TestWorkbookParsesBasicConstantsAndIgnoresNoOp(t *testing.T) {
	f := excelize.NewFile()
	path := filepath.Join(t.TempDir(), "constants.xlsx")
	if err := f.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	wb, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer wb.Close()
	if err := wb.SetCellText("Sheet1", "A1", ""); err != nil {
		t.Fatal(err)
	}
	if wb.Dirty() {
		t.Fatal("setting an empty cell to empty should be a no-op")
	}
	for _, item := range []struct{ cell, value, want string }{
		{"A1", "42.5", "42.5"},
		{"A2", "TRUE", "TRUE"},
		{"A3", "普通文本", "普通文本"},
	} {
		if err := wb.SetCellText("Sheet1", item.cell, item.value); err != nil {
			t.Fatal(err)
		}
		got, err := wb.Value("Sheet1", item.cell)
		if err != nil || got != item.want {
			t.Fatalf("%s value = %q, want %q, err = %v", item.cell, got, item.want, err)
		}
	}
	if err := wb.SetCellText("Sheet1", "A4", "Inf"); err == nil {
		t.Fatal("expected non-finite numeric input to be rejected")
	}
}

func TestOfficeXlsxSamplesOpenAndSaveWithoutChangingSource(t *testing.T) {
	dir := os.Getenv("GLANCE_OFFICE_TEST_DIR")
	if dir == "" {
		t.Skip("set GLANCE_OFFICE_TEST_DIR to the local office sample directory")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || filepath.Ext(entry.Name()) != ".xlsx" {
			continue
		}
		count++
		path := filepath.Join(dir, entry.Name())
		original, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		wb, err := Open(path)
		if err != nil {
			t.Errorf("open %s: %v", entry.Name(), err)
			continue
		}
		if len(wb.SheetNames()) == 0 {
			t.Errorf("%s contains no visible worksheet names", entry.Name())
			_ = wb.Close()
			continue
		}
		beforeNames := wb.SheetNames()
		beforeRanges := make(map[string][][]document.Cell, len(beforeNames))
		for _, name := range beforeNames {
			preview, err := wb.Range(name, 300, 16)
			if err != nil {
				t.Errorf("read preview of %s/%s: %v", entry.Name(), name, err)
				continue
			}
			beforeRanges[name] = preview
		}
		if entry.Name() == "人事表格-员工绩效表.xlsx" {
			preview := beforeRanges["月绩效"]
			if len(preview) < 10 || preview[0][0].Value == "" || preview[3][0].Value != "考核项" {
				t.Errorf("human-resources sample did not expose expected visible cells: rows=%d A1=%q A4=%q", len(preview), preview[0][0].Value, preview[3][0].Value)
			}
			nonEmpty := 0
			for _, row := range preview {
				for _, cell := range row {
					if cell.Value != "" {
						nonEmpty++
					}
				}
			}
			if nonEmpty < 20 {
				t.Errorf("human-resources sample exposed only %d non-empty cells", nonEmpty)
			}
		}
		copyPath := filepath.Join(t.TempDir(), entry.Name())
		if err := wb.SaveAs(copyPath); err != nil {
			t.Errorf("save copy of %s: %v", entry.Name(), err)
		}
		if err := wb.Close(); err != nil {
			t.Errorf("close %s: %v", entry.Name(), err)
		}
		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(after) != string(original) {
			t.Errorf("opening/saving a copy modified source file %s", entry.Name())
		}
		copyBook, err := Open(copyPath)
		if err != nil {
			t.Errorf("reopen copy of %s: %v", entry.Name(), err)
			continue
		}
		if !reflect.DeepEqual(copyBook.SheetNames(), beforeNames) {
			t.Errorf("sheet names changed in copy of %s", entry.Name())
		}
		for name, before := range beforeRanges {
			after, err := copyBook.Range(name, 300, 16)
			if err != nil || !reflect.DeepEqual(after, before) {
				t.Errorf("preview changed after save/reopen for %s/%s (err=%v)", entry.Name(), name, err)
			}
		}
		_ = copyBook.Close()
	}
	if count == 0 {
		t.Fatal("no .xlsx samples found in GLANCE_OFFICE_TEST_DIR")
	}
}
