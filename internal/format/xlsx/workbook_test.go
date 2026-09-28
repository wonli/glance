package xlsx

import (
	"os"
	"path/filepath"
	"testing"

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
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".xlsx" {
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
		if err := wb.SaveAs(filepath.Join(t.TempDir(), entry.Name())); err != nil {
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
	}
	if count == 0 {
		t.Fatal("no .xlsx samples found in GLANCE_OFFICE_TEST_DIR")
	}
}
