package xlsx

import (
	"errors"
	excelize "github.com/xuri/excelize/v2"
	"glance/internal/document"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestWorkbookSaveAsWithCommitGuard(t *testing.T) {
	book := NewBlank()
	defer book.Close()
	if err := book.SetCellText("Sheet1", "A1", "original"); err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(t.TempDir(), "copy.xlsx")

	guardErr := errors.New("guard aborted")
	err := book.SaveAsWithCommitGuard(target, func() (func(), error) {
		return nil, guardErr
	})
	if !errors.Is(err, guardErr) {
		t.Fatalf("expected guardErr, got %v", err)
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Fatalf("target should not exist after guard failure")
	}

	calledRelease := false
	err = book.SaveAsWithCommitGuard(target, func() (func(), error) {
		return func() { calledRelease = true }, nil
	})
	if err != nil {
		t.Fatalf("SaveAsWithCommitGuard failed: %v", err)
	}
	if !calledRelease {
		t.Fatal("release was not called after successful save")
	}
	if _, statErr := os.Stat(target); statErr != nil {
		t.Fatalf("target should exist after successful save: %v", statErr)
	}
}

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

func TestWorkbookFailedSaveKeepsDirtyAndOriginal(t *testing.T) {
	f := excelize.NewFile()
	if err := f.SetCellValue("Sheet1", "A1", "original"); err != nil {
		t.Fatal(err)
	}
	path := saveTestWorkbook(t, f)
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	wb, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer wb.Close()
	if err := wb.SetCellText("Sheet1", "A1", "edited"); err != nil {
		t.Fatal(err)
	}
	backup := path + ".backup"
	if err := os.Rename(path, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		_ = os.Rename(backup, path)
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(path)
		_ = os.Rename(backup, path)
	}()
	if err := wb.Save(); err == nil {
		t.Fatal("Save should fail when the destination path is a directory")
	}
	if !wb.Dirty() {
		t.Fatal("failed save should retain dirty state")
	}
	after, err := os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, original) {
		t.Fatal("failed save changed original workbook bytes")
	}
}
