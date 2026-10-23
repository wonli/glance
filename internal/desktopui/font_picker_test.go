package desktopui

import (
	"glance/internal/application"
	"glance/internal/fontmanager/fontcatalog"
	"testing"
)

func TestFontPickerDocumentFontsFirst(t *testing.T) {
	s := fontcatalog.NewSnapshot(1, []fontcatalog.Face{{ID: "1", Family: "Go", Source: fontcatalog.System}, {ID: "2", Family: "Arimo", Source: fontcatalog.System}})
	rows := fontPickerRows(s, []string{"Go", "Missing"}, nil, "")
	if len(rows) < 3 || rows[1].family != "Go" || rows[2].family != "Missing" || !rows[2].missing {
		t.Fatalf("rows: %+v", rows)
	}
}
func TestFontPickerEmptyProviderNoPlaceholder(t *testing.T) {
	rows := fontPickerRows(fontcatalog.NewSnapshot(1, []fontcatalog.Face{{ID: "1", Family: "Go"}}), nil, nil, "")
	for _, r := range rows {
		if r.title == "更多字体" || r.online != nil {
			t.Fatalf("unexpected online placeholder: %+v", rows)
		}
	}
}
func TestFontPickerSearchIMEAndClear(t *testing.T) {
	s := fontcatalog.NewSnapshot(1, []fontcatalog.Face{{ID: "1", Family: "Go", Aliases: []string{"测试字体"}}})
	if len(fontPickerRows(s, nil, nil, "测试")) != 2 {
		t.Fatal("localized search failed")
	}
	if len(fontPickerRows(s, nil, nil, "unknown")) != 0 {
		t.Fatal("search returned unrelated fonts")
	}
}
func TestFontPickerRejectsStaleSelection(t *testing.T) {
	a := &App{documentSessionState: newDocumentSessionState("one.docx")}
	a.fontPicker.open = true
	a.fontPicker.target.state = a.documentSessionState
	a.documentSessionState = newDocumentSessionState("two.docx")
	if a.fontPickerTargetValid() {
		t.Fatal("stale document accepted")
	}
}

func TestFontPickerNoOpDoesNotModifyDocument(t *testing.T) {
	a := &App{documentSessionState: newDocumentSessionState("x.xlsx")}
	a.fontPicker.target = fontPickerTarget{state: a.documentSessionState, current: "Go"}
	if a.applyFontChoice("Go") {
		t.Fatal("accepted missing spreadsheet target")
	}
}

func TestFontRefreshDoesNotCancelActiveExport(t *testing.T) {
	a := New(nil, "")
	a.initFontPicker()
	s := newDocumentSessionState("x.docx")
	cancelled := false
	s.wordPDFExportCancel = func() { cancelled = true }
	a.documentSessionState = s
	a.sessions = map[string]*documentSessionState{"x": s}
	a.fontPicker.events <- application.FontEvent{Snapshot: fontcatalog.NewSnapshot(1, nil)}
	a.pollFontResults()
	a.stopAndWaitManagedBackground()
	if cancelled {
		t.Fatal("font refresh cancelled active PDF export")
	}
}

func TestFontPickerPreservesSourceConflicts(t *testing.T) {
	s := fontcatalog.NewSnapshot(1, []fontcatalog.Face{{ID: "system", Family: "Go", Source: fontcatalog.System}, {ID: "private", Family: "Go", Source: fontcatalog.Private}})
	rows := fontPickerRows(s, nil, nil, "")
	if len(rows) != 3 || rows[1].faceID == rows[2].faceID {
		t.Fatalf("conflicting font sources collapsed: %+v", rows)
	}
}
