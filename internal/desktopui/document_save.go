package desktopui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"glance/internal/application"
	"glance/internal/document"
	"glance/internal/workspace"
	"path/filepath"
	"strings"
)

type wordSaveReason uint8

const (
	wordSaveManual wordSaveReason = iota
	wordSaveAuto
)

func (reason wordSaveReason) historySource() workspace.WordVersionSource {
	if reason == wordSaveAuto {
		return workspace.WordVersionAutoSave
	}
	return workspace.WordVersionManualSave
}

type saveResult struct {
	path       string
	tabID      string
	saveAs     bool
	token      sessionOperationToken
	err        error
	historyErr error
}

func (a *App) startSave() {
	a.startSaveForReason(wordSaveManual)
}

func (a *App) startSaveForReason(reason wordSaveReason) {
	if !a.prepareMarkdownSave() {
		return
	}
	if state := a.documentSessionState; a.word != nil && state != nil && (state.wordParagraphSplitIntent != nil || state.wordParagraphJoinIntent != nil) {
		state.wordSaveAfterDrafts = true
		state.wordSaveAfterDraftsReason = reason
		state.wordForceDraftFlush.Store(true)
		a.requestWordTextFlush(true)
		return
	}
	if a.word != nil && wordHasPendingTextDrafts(a.documentSessionState) {
		a.documentSessionState.wordSaveAfterDrafts = true
		a.documentSessionState.wordSaveAfterDraftsReason = reason
		a.documentSessionState.wordForceDraftFlush.Store(true)
		a.requestWordTextFlush(true)
		return
	}
	if a.isSaving() {
		return
	}
	if a.pptxDoc != nil {
		if err := a.flushPresentationNotesDraft(a.pptxView); err != nil {
			return
		}
		if err := a.flushPresentationTextDraft(); err != nil {
			return
		}
	}
	if !a.isDirty() {
		return
	}
	state := a.documentSessionState
	if state == nil {
		return
	}
	if state.saveAsRequired {
		a.startSaveAsPicker("xlsx")
		return
	}
	token, ok := state.beginOperation(sessionOperationSave)
	if !ok {
		return
	}
	book, word, pptxDocument, jsonDocument, markdownDocument, pdfDocument, tabID := a.book, a.word, a.pptxDoc, a.jsonDoc, a.markdownDoc, a.pdfDoc, a.activeTabID
	path := sessionPath(state)
	a.saving = true
	a.launchManagedBackground(func() {
		var err error
		var historyErr error
		var snapshot []byte
		applicationRuntime := a.ensureDesktopRuntime().applicationRuntime
		if applicationRuntime != nil {
			_, snapshot, err = applicationRuntime.SaveDocumentWithSnapshot(context.Background(), state.documentID)
			if errors.Is(err, application.ErrDocumentNotFound) || errors.Is(err, application.ErrInvalidDocumentID) {
				err = nil
				applicationRuntime = nil
			}
		}
		if applicationRuntime == nil {
			unlock := lockSessionAdapterWrite(state)
			switch {
			case book != nil:
				err = book.Save()
			case word != nil:
				if saver, ok := any(word).(document.WordPackageSnapshotSaver); ok {
					snapshot, err = saver.SaveWithSnapshot()
				} else {
					err = word.Save()
				}
			case pptxDocument != nil:
				err = pptxDocument.Save()
			case pdfDocument != nil:
				err = pdfDocument.Save()
			case markdownDocument != nil:
				err = markdownDocument.Save()
			case jsonDocument != nil:
				err = jsonDocument.Save()
			}
			unlock()
		}
		state.finishOperation(token)
		if err == nil && word != nil && snapshot != nil {
			historyErr = a.ensureDesktopRuntime().addWordSnapshot(path, reason.historySource(), snapshot)
		}
		select {
		case a.saveResults <- saveResult{path: path, tabID: tabID, token: token, err: err, historyErr: historyErr}:
			if a.window != nil {
				a.window.Invalidate()
			}
		case <-a.shutdown:
		}
	})
}

func (a *App) startSaveAs(path string) {
	if !a.prepareMarkdownSave() {
		return
	}
	if a.isSaving() {
		return
	}
	format := "xlsx"
	if a.word != nil {
		format = "docx"
	}
	if a.pptxDoc != nil {
		format = "pptx"
	}
	if a.markdownDoc != nil {
		format = "markdown"
	}
	if a.pdfDoc != nil {
		format = "pdf"
	}
	if a.jsonDoc != nil {
		format = "json"
	}
	if err := validateSaveAsExtension(format, path); err != nil {
		a.setStatus(err.Error())
		if a.pendingCloseTabID != "" {
			a.showCloseConfirmation(a.pendingCloseTabID, "保存失败", "文件扩展名不匹配。可以重试保存、不保存并关闭，或取消关闭。")
		}
		return
	}
	if a.pptxDoc != nil {
		if err := a.flushPresentationNotesDraft(a.pptxView); err != nil {
			return
		}
		if err := a.flushPresentationTextDraft(); err != nil {
			return
		}
	}
	state := a.documentSessionState
	if state == nil {
		a.setStatus("没有活动文档")
		return
	}
	token, ok := state.beginOperation(sessionOperationSave)
	if !ok {
		return
	}
	book, word, pptxDocument, jsonDocument, markdownDocument, pdfDocument, tabID := a.book, a.word, a.pptxDoc, a.jsonDoc, a.markdownDoc, a.pdfDoc, a.activeTabID
	a.saving = true
	a.launchManagedBackground(func() {
		if pdfDocument != nil {
			err := a.savePDFAsThroughRuntime(state, token, path)
			state.finishOperation(token)
			select {
			case a.saveResults <- saveResult{path: path, tabID: tabID, saveAs: true, token: token, err: err}:
				if a.window != nil {
					a.window.Invalidate()
				}
			case <-a.shutdown:
			}
			return
		}
		unlock := lockSessionAdapterWrite(state)
		var err error
		var historyErr error
		switch {
		case book != nil:
			if saver, ok := book.(interface{ SaveAsDocument(string) error }); ok {
				err = saver.SaveAsDocument(path)
			} else {
				err = fmt.Errorf("当前表格格式不支持另存为")
			}
		case word != nil:
			var snapshot []byte
			if saver, ok := any(word).(document.WordPackageSnapshotSaver); ok {
				snapshot, err = saver.SaveAsDocumentWithSnapshot(path)
			} else {
				err = word.SaveAsDocument(path)
			}
			unlock()
			if err == nil {
				a.ensureDesktopRuntime().updateSessionPath(state.sharedDocumentSession, path)
			}
			state.finishOperation(token)
			if err == nil && snapshot != nil {
				historyErr = a.ensureDesktopRuntime().addWordSnapshot(path, workspace.WordVersionManualSave, snapshot)
			}
			select {
			case a.saveResults <- saveResult{path: path, tabID: tabID, saveAs: true, token: token, err: err, historyErr: historyErr}:
				if a.window != nil {
					a.window.Invalidate()
				}
			case <-a.shutdown:
			}
			return
		case pptxDocument != nil:
			err = pptxDocument.SaveAs(path)
			unlock()
			if err == nil {
				a.ensureDesktopRuntime().updateSessionPath(state.sharedDocumentSession, path)
			}
			state.finishOperation(token)
			select {
			case a.saveResults <- saveResult{path: path, tabID: tabID, saveAs: true, token: token, err: err}:
				if a.window != nil {
					a.window.Invalidate()
				}
			case <-a.shutdown:
			}
			return
		case markdownDocument != nil:
			err = markdownDocument.SaveAs(path)
		case jsonDocument != nil:
			err = jsonDocument.SaveAs(path)
		default:
			err = fmt.Errorf("没有活动文档")
		}
		unlock()
		state.finishOperation(token)
		select {
		case a.saveResults <- saveResult{path: path, tabID: tabID, saveAs: true, token: token, err: err, historyErr: historyErr}:
			if a.window != nil {
				a.window.Invalidate()
			}
		case <-a.shutdown:
		}
	})
}

func (a *App) savePDFAsThroughRuntime(state *documentSessionState, token sessionOperationToken, path string) error {
	runtime := a.ensureDesktopRuntime()
	if runtime.applicationRuntimeErr != nil {
		return runtime.applicationRuntimeErr
	}
	if runtime.applicationRuntime == nil {
		return application.ErrRuntimeClosed
	}
	arguments, err := json.Marshal(struct {
		Path string `json:"path"`
	}{path})
	if err != nil {
		return err
	}
	_, err = runtime.applicationRuntime.InvokeCapability(context.Background(), application.CapabilityRequest{
		RequestID:  fmt.Sprintf("pdf-save-as-%s-%d", token.documentID, token.generation),
		DocumentID: token.documentID, CapabilityID: "document.save_as", ExpectedRevision: token.revision,
		Operator:  application.Operator{ID: fmt.Sprintf("window-%d", a.windowID), Kind: application.OperatorHuman},
		Arguments: arguments,
	})
	return err
}

func validateSaveAsExtension(format, path string) error {
	if format == "markdown" {
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".md" || ext == ".markdown" {
			return nil
		}
		return fmt.Errorf("另存为扩展名必须是 .md 或 .markdown")
	}
	want := map[string]string{"xlsx": ".xlsx", "docx": ".docx", "json": ".json", "pptx": ".pptx", "pdf": ".pdf"}[format]
	if want == "" {
		return fmt.Errorf("不支持 %s 格式另存为", format)
	}
	if !strings.EqualFold(filepath.Ext(path), want) {
		return fmt.Errorf("另存为扩展名必须是 %s", want)
	}
	return nil
}

func (a *App) save() error {
	if !a.prepareMarkdownSave() {
		return fmt.Errorf("请先完成输入法选字")
	}
	unlock := lockSessionAdapterWrite(a.documentSessionState)
	defer unlock()
	switch {
	case a.book != nil:
		if err := a.book.Save(); err != nil {
			return err
		}
	case a.word != nil:
		if err := a.word.Save(); err != nil {
			return err
		}
	case a.pdfDoc != nil:
		return a.pdfDoc.Save()
	case a.markdownDoc != nil:
		return a.markdownDoc.Save()
	case a.jsonDoc != nil:
		if a.path == "" {
			return fmt.Errorf("JSON 草稿需要另存为")
		}
		if err := a.jsonDoc.Save(); err != nil {
			return err
		}
	case a.pptxDoc != nil:
		if err := a.flushPresentationTextDraft(); err != nil {
			return err
		}
		return a.pptxDoc.Save()
	default:
		return fmt.Errorf("没有打开的 Office 文件")
	}
	return nil
}
