package desktopui

import (
	"reflect"
	"testing"
	"time"

	"glance/internal/document"
	"glance/internal/format/docx"
	"glance/internal/i18n"
)

func TestWordDocumentInfoRibbonOpensAndCommitsCoreProperties(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	state := newDocumentSessionState("info.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	app := &App{
		documentSessionState:    state,
		wordCommandResults:      make(chan wordCommandResult, 1),
		wordDocumentInfoResults: make(chan wordDocumentInfoResult, 1),
	}
	app.applyLocaleSelection(i18n.SelectionChinese)

	action := findWordFileTestAction(t, "doc-info")
	if !app.wordToolbarActionEnabled(action) {
		t.Fatal("Word File > Document Info should be available for a DOCX")
	}
	app.executeWordToolbarAction("doc-info")
	deadline := time.Now().Add(3 * time.Second)
	for !app.modal.Visible && time.Now().Before(deadline) {
		app.processAsyncResults()
		time.Sleep(5 * time.Millisecond)
	}
	if !app.modal.Visible || app.text(app.modal.TitleRef) != "文档信息" {
		t.Fatalf("Document Info opened modal=%v title=%q", app.modal.Visible, app.text(app.modal.TitleRef))
	}

	inputs := reflect.ValueOf(&app.modal).Elem().FieldByName("FormInputs")
	if !inputs.IsValid() || inputs.Len() != 5 {
		t.Fatal("Document Info form should expose five editable core-property fields")
	}
	for index, value := range []string{"标题 A<&", "主题", "作者", "关键词", "说明"} {
		editor := inputs.Index(index).Addr().MethodByName("SetText")
		if !editor.IsValid() {
			t.Fatalf("Document Info field %d is not an editable Gio editor", index)
		}
		editor.Call([]reflect.Value{reflect.ValueOf(value)})
	}
	if app.modal.OnAction == nil {
		t.Fatal("Document Info form has no submit handler")
	}
	app.modal.OnAction("confirm")
	if app.modal.Visible {
		t.Fatal("submitting Document Info did not close the dialog")
	}
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatal(result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Document Info to commit")
	}

	getter := reflect.ValueOf(word).MethodByName("GetWordDocumentProperties")
	if !getter.IsValid() {
		t.Fatal("DOCX adapter does not expose GetWordDocumentProperties")
	}
	values := getter.Call(nil)
	if !values[1].IsNil() {
		t.Fatalf("reading committed Word properties: %v", values[1].Interface())
	}
	properties := values[0]
	for name, want := range map[string]string{"Title": "标题 A<&", "Subject": "主题", "Creator": "作者", "Keywords": "关键词", "Description": "说明"} {
		field := properties.FieldByName(name)
		if !field.IsValid() || field.String() != want {
			t.Errorf("Document Info %s = %v; want %q", name, field, want)
		}
	}
}

func TestWordDocumentInfoPrefillsValuesAndCancelDoesNotWrite(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	want := map[string]string{"Title": "已保存标题", "Subject": "既有主题", "Creator": "作者", "Keywords": "词一;词二", "Description": "已有说明"}
	properties := document.WordDocumentProperties{Title: want["Title"], Subject: want["Subject"], Creator: want["Creator"], Keywords: want["Keywords"], Description: want["Description"]}
	if err := word.SetWordDocumentProperties(properties); err != nil {
		t.Fatal(err)
	}
	word.SetDirty(false)
	state := newDocumentSessionState("info.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	app := &App{
		documentSessionState:    state,
		wordCommandResults:      make(chan wordCommandResult, 1),
		wordDocumentInfoResults: make(chan wordDocumentInfoResult, 1),
	}
	app.executeWordToolbarAction("doc-info")
	deadline := time.Now().Add(3 * time.Second)
	for !app.modal.Visible && time.Now().Before(deadline) {
		app.processAsyncResults()
		time.Sleep(5 * time.Millisecond)
	}
	if !app.modal.Visible {
		t.Fatal("Document Info did not load existing properties")
	}
	for index, name := range []string{"Title", "Subject", "Creator", "Keywords", "Description"} {
		if got := app.modal.FormInputs[index].Text(); got != want[name] {
			t.Errorf("Document Info initial %s=%q; want %q", name, got, want[name])
		}
	}
	app.modal.FormInputs[0].SetText("取消时丢弃")
	app.modal.OnAction("cancel")
	got, err := word.GetWordDocumentProperties()
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != want["Title"] || got.Subject != want["Subject"] || got.Creator != want["Creator"] || got.Keywords != want["Keywords"] || got.Description != want["Description"] {
		t.Fatalf("cancel changed document properties: %+v", got)
	}
}
