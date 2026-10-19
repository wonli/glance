package desktopui

import (
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"glance/internal/document"
	"glance/internal/format/markdowndoc"
	wordpdf "glance/internal/format/pdf"
	"glance/internal/i18n"
	"glance/internal/wordlayout"
)

type markdownFileResult struct {
	state                    *documentSessionState
	source, sourcePath, path string
	revision, draftEpoch     uint64
	picker                   bool
	preflight                bool
	orientation              string
	prepared                 *markdownPDFPrepared
	warnings                 string
	missingImages            int
	err                      error
}

type markdownPDFPrepared struct {
	blocks []document.WordBlock
	scene  wordlayout.Result
	shaper *wordTextShaper
	images wordpdf.MarkdownImages
}

func prepareMarkdownPDF(ctx context.Context, snapshot document.MarkdownSnapshot, resources map[string]image.Image, diagnostics map[string]string) (*markdownPDFPrepared, error) {
	blocks, images, err := wordpdf.MarkdownWordBlocks(snapshot, resources, diagnostics)
	if err != nil {
		return nil, err
	}
	shaper, err := newWordTextShaper()
	if err != nil {
		return nil, err
	}
	scene, err := wordlayout.NewEngine(shaper).Layout(ctx, wordlayout.Input{DocumentID: "markdown-pdf", Blocks: blocks, DefaultPageSetup: defaultWordPageSetup})
	if err != nil {
		return nil, err
	}
	for _, diagnostic := range scene.Diagnostics {
		if diagnostic.Severity == wordlayout.DiagnosticError {
			return nil, fmt.Errorf("Markdown PDF: %s", diagnostic.Detail)
		}
	}
	// Validate serialization and font permissions before showing the save picker.
	if err := wordpdf.WriteWordPDF(ctx, io.Discard, scene, shaper, images); err != nil {
		return nil, err
	}
	return &markdownPDFPrepared{blocks: blocks, scene: scene, shaper: shaper, images: images}, nil
}

var markdownPDFDestinations = struct {
	sync.Mutex
	active map[string]bool
}{active: map[string]bool{}}

func exportMarkdownPDF(ctx context.Context, snapshot document.MarkdownSnapshot, sourcePath, path string, guard func() (func(), error)) error {
	return exportMarkdownPDFWithImageWarnings(ctx, snapshot, sourcePath, path, guard, nil)
}

func exportMarkdownPDFWithImageWarnings(ctx context.Context, snapshot document.MarkdownSnapshot, sourcePath, path string, guard func() (func(), error), missingImages *int) error {
	resources, diagnostics := markdowndoc.PreviewResourcesContext(ctx, snapshot, sourcePath)
	count, _ := markdownImageWarnings(snapshot, resources, diagnostics)
	if missingImages != nil {
		*missingImages = count
	}
	prepared, err := prepareMarkdownPDF(ctx, snapshot, resources, diagnostics)
	if err != nil {
		return err
	}
	return exportPreparedMarkdownPDF(ctx, prepared, sourcePath, path, guard)
}

func exportPreparedMarkdownPDF(ctx context.Context, prepared *markdownPDFPrepared, sourcePath, path string, guard func() (func(), error)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if markdownPDFSameFile(path, sourcePath) {
		return errors.New("PDF 保存路径不能覆盖源 Markdown")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	key := strings.ToLower(filepath.Clean(absolute))
	markdownPDFDestinations.Lock()
	if markdownPDFDestinations.active[key] {
		markdownPDFDestinations.Unlock()
		return errors.New("该 PDF 目标正在导出")
	}
	markdownPDFDestinations.active[key] = true
	markdownPDFDestinations.Unlock()
	defer func() {
		markdownPDFDestinations.Lock()
		delete(markdownPDFDestinations.active, key)
		markdownPDFDestinations.Unlock()
	}()
	return wordpdf.ExportWordPDFFileWithCommitGuard(ctx, path, prepared.scene, prepared.shaper, prepared.images, guard)
}

func markdownPDFSameFile(left, right string) bool {
	if pathsReferToSameFile(left, right) {
		return true
	}
	l, le := os.Stat(left)
	r, re := os.Stat(right)
	return le == nil && re == nil && os.SameFile(l, r)
}

func (a *App) requestMarkdownPDFExport() {
	state := a.documentSessionState
	if state == nil || a.markdownDoc == nil || state.closed.Load() || state.formatRunning.Load() || a.isSaving() || a.fileCommandRunning {
		return
	}
	view := a.ensureMarkdownView()
	if view.exportPending {
		return
	}
	if markdownEditorComposing(&view.editor) {
		a.setMarkdownExportStatus(i18n.Message{ID: "markdown.pdf.export.error.finish_composition"}, feedbackWarning, notificationToast, "")
		return
	}
	a.flushMarkdownDraft()
	source := view.editor.Text()
	if a.markdownDoc.Text() != source {
		a.setMarkdownExportStatus(i18n.Message{ID: "markdown.pdf.export.error.pending_edit"}, feedbackWarning, notificationToast, "")
		return
	}
	result := markdownFileResult{state: state, source: source, sourcePath: a.path, revision: state.wordRevisionAtomic.Load(), draftEpoch: view.draftEpoch.Load(), preflight: true}
	view.exportPending = true
	view.lastPDFPath = ""
	shutdown, window := a.shutdown, a.window
	var resources map[string]image.Image
	var diagnostics map[string]string
	if !view.pending && view.snapshot.Source == source && view.parsedPath == a.path && view.images != nil {
		resources, diagnostics = maps.Clone(view.images), maps.Clone(view.errors)
	}
	a.launchManagedBackground(func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan struct{})
		defer close(done)
		a.launchManagedBackground(func() {
			select {
			case <-shutdown:
				cancel()
			case <-done:
			}
		})
		snapshot, err := markdowndoc.Parse(source)
		if err == nil {
			if resources == nil {
				resources, diagnostics = markdowndoc.PreviewResourcesContext(ctx, snapshot, result.sourcePath)
			}
			result.missingImages, result.warnings = markdownImageWarnings(snapshot, resources, diagnostics, a.text)
			result.prepared, err = prepareMarkdownPDF(ctx, snapshot, resources, diagnostics)
		}
		result.err = err
		select {
		case view.exports <- result:
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
		}
	})
	a.setMarkdownExportStatus(i18n.Message{ID: "markdown.pdf.export.status.checking"}, feedbackInfo, notificationSuppressed, "")
}

func (a *App) startMarkdownPDFPicker(result markdownFileResult) {
	view := a.ensureMarkdownView()
	if result.state != a.documentSessionState || result.state.closed.Load() || result.source != view.editor.Text() || result.revision != result.state.wordRevisionAtomic.Load() || result.draftEpoch != view.draftEpoch.Load() || result.sourcePath != a.path {
		view.exportPending = false
		a.setMarkdownExportStatus(i18n.Message{ID: "markdown.pdf.export.error.document_changed"}, feedbackWarning, notificationToast, "")
		return
	}
	result.preflight, result.picker = false, true
	view.exportPending = true
	runner, shutdown, window := a.fileCommandRunner, a.shutdown, a.window
	pickerLabels := a.filePickerLabels()
	a.launchManagedBackground(func() {
		if runner != nil {
			picked := runner(commandExportPDF, "pdf")
			result.path, result.err = picked.path, picked.err
		} else {
			result.path, result.err = chooseSavePath("pdf", pickerLabels)
		}
		select {
		case view.exports <- result:
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
		}
	})
	a.setMarkdownExportStatus(i18n.Message{ID: "markdown.pdf.export.status.choose_path"}, feedbackInfo, notificationSuppressed, "")
}

func (a *App) pollMarkdownFileResults() {
	view := a.markdownView
	if view == nil {
		return
	}
	select {
	case result := <-view.exports:
		if result.orientation != "" {
			prev := &a.pdfExportPreview
			if !prev.Active || prev.Format != "markdown" || prev.markdownResult.prepared == nil || prev.markdownResult.prepared.shaper != result.prepared.shaper {
				return
			}
		}
		view.exportPending = false
		if result.state != a.documentSessionState || result.state.closed.Load() {
			return
		}
		if result.source != view.editor.Text() || result.revision != result.state.wordRevisionAtomic.Load() || result.draftEpoch != view.draftEpoch.Load() || result.sourcePath != a.path {
			a.setMarkdownExportStatus(i18n.Message{ID: "markdown.pdf.export.error.document_changed"}, feedbackWarning, notificationToast, "")
			return
		}
		if result.err != nil && result.orientation == "" {
			a.setMarkdownExportStatus(i18n.Message{ID: "markdown.pdf.export.error.failed"}, feedbackError, notificationToast, result.err.Error())
			if result.preflight {
				a.modal.ShowMessage("markdown.pdf.export.preflight_failed", i18n.Message{ID: "markdown.pdf.export.preflight_failed.title"}, i18n.Message{ID: "markdown.pdf.export.preflight_failed.body"}, func(string) { a.modal.Hide() }, modalDialogActionSpec{ID: "close", LabelRef: i18n.Message{ID: "common.action.close"}})
			}
			return
		}
		if result.orientation != "" {
			a.applyMarkdownPDFOrientation(result)
			return
		}
		if result.preflight {
			a.openMarkdownPDFExportPreview(result)
			return
		}
		if !result.picker {
			info, err := os.Stat(result.path)
			if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
				a.setMarkdownExportStatus(i18n.Message{ID: "markdown.pdf.export.error.output_missing"}, feedbackError, notificationToast, result.path)
				return
			}
			view.lastPDFPath = result.path
			message := i18n.Message{ID: "markdown.pdf.export.status.exported", Args: i18n.Args{"Path": result.path}}
			if result.missingImages > 0 {
				message = i18n.Message{ID: "markdown.pdf.export.status.exported_with_warnings", Args: i18n.Args{"Count": result.missingImages, "Path": result.path}}
			}
			a.setMarkdownExportStatus(message, feedbackSuccess, notificationToast, "")
			return
		}
		if result.path == "" {
			a.setMarkdownExportStatus(i18n.Message{ID: "markdown.pdf.export.status.cancelled"}, feedbackWarning, notificationSuppressed, "")
			return
		}
		if strings.ToLower(filepath.Ext(result.path)) != ".pdf" {
			result.path += ".pdf"
		}
		absolute, err := filepath.Abs(result.path)
		if err != nil {
			a.setMarkdownExportStatus(i18n.Message{ID: "markdown.pdf.export.error.path"}, feedbackError, notificationToast, err.Error())
			return
		}
		result.path = absolute
		if markdownPDFSameFile(result.path, result.sourcePath) {
			a.setMarkdownExportStatus(i18n.Message{ID: "markdown.pdf.export.error.overwrite_source"}, feedbackWarning, notificationToast, "")
			return
		}
		view.exportPending = true
		result.picker = false
		shutdown, window := a.shutdown, a.window
		doc := a.markdownDoc
		a.launchManagedBackground(func() {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan struct{})
			defer close(done)
			a.launchManagedBackground(func() {
				select {
				case <-shutdown:
					cancel()
				case <-done:
				}
			})
			err := exportPreparedMarkdownPDF(ctx, result.prepared, result.sourcePath, result.path, func() (func(), error) {
				release := lockSessionAdapterRead(result.state)
				if ctx.Err() != nil {
					release()
					return nil, ctx.Err()
				}
				if result.state.closed.Load() || result.state.wordRevisionAtomic.Load() != result.revision || view.draftEpoch.Load() != result.draftEpoch || doc.Text() != result.source {
					release()
					return nil, errors.New("Markdown 文档已更改，未覆盖目标 PDF")
				}
				return release, nil
			})
			result.err = err
			select {
			case view.exports <- result:
				if window != nil {
					window.Invalidate()
				}
			case <-shutdown:
			}
		})
		a.setMarkdownExportStatus(i18n.Message{ID: "markdown.pdf.export.status.generating"}, feedbackInfo, notificationSuppressed, "")
	default:
	}
}

func (a *App) setMarkdownExportStatus(message i18n.Message, kind feedbackKind, policy notificationPolicy, detail string) {
	view := a.ensureMarkdownView()
	view.exportMessage = message
	if kind == feedbackError {
		view.lastPDFPath = ""
	}
	if detail != "" {
		a.setStatusDetailMessage(message, kind, policy, detail)
	} else {
		a.setStatusMessage(message, kind, policy)
	}
}
