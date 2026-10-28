package desktopui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"glance/internal/application"
)

type pdfMergeSplitJobKind uint8

const (
	pdfMergeSplitJobMergeFile pdfMergeSplitJobKind = iota
	pdfMergeSplitJobSplitSource
	pdfMergeSplitJobMergeFolder
	pdfMergeSplitJobSplitFolder
	pdfMergeSplitJobOperation
)

type pdfMergeSplitJobResult struct {
	generation uint64
	mode       pdfMergeSplitMode
	kind       pdfMergeSplitJobKind
	file       application.PDFUtilityFileInfo
	files      []application.PDFUtilityFileInfo
	folder     string
	outputs    []application.PDFUtilityOutput
	err        error
}

func (a *App) startPDFMergeSplitFilePicker(kind pdfMergeSplitJobKind) {
	state := &a.pdfMergeSplitTool
	if state.busy {
		return
	}
	mode := state.mode
	var picker func(context.Context, filePickerLabels) (string, error)
	var multiPicker func(context.Context, filePickerLabels) ([]string, error)
	if kind == pdfMergeSplitJobMergeFile {
		picker = state.mergeDraft.pickPDFFile
		multiPicker = state.mergeDraft.pickPDFFiles
		if picker == nil && multiPicker == nil {
			multiPicker = chooseOfficeFilesContext
		}
	} else {
		picker = state.splitDraft.pickPDFFile
		if picker == nil {
			picker = chooseOfficeFileContext
		}
	}
	labels := a.filePickerLabels()
	labels.AllowedExtensions = []string{"pdf"}
	runtime := a.ensureDesktopRuntime().applicationRuntime
	runtimeErr := a.ensureDesktopRuntime().applicationRuntimeErr
	a.startPDFMergeSplitJob(kind, mode, func(ctx context.Context) pdfMergeSplitJobResult {
		result := pdfMergeSplitJobResult{kind: kind}
		var paths []string
		var err error
		if multiPicker != nil {
			paths, err = multiPicker(ctx, labels)
		} else {
			var path string
			path, err = picker(ctx, labels)
			if path != "" {
				paths = []string{path}
			}
		}
		if err != nil || len(paths) == 0 {
			result.err = err
			return result
		}
		if runtime == nil {
			result.err = runtimeErr
			if result.err == nil {
				result.err = errors.New("PDF services are unavailable")
			}
			return result
		}
		if kind == pdfMergeSplitJobMergeFile {
			files := make([]application.PDFUtilityFileInfo, 0, len(paths))
			for _, path := range paths {
				file, inspectErr := runtime.InspectPDFUtilityFile(ctx, path)
				if inspectErr != nil {
					result.err = fmt.Errorf("%s: %w", filepath.Base(path), inspectErr)
					return result
				}
				files = append(files, file)
			}
			result.files = files
			return result
		}
		result.file, result.err = runtime.InspectPDFUtilityFile(ctx, paths[0])
		return result
	})
}

func (a *App) startPDFMergeSplitFolderPicker(kind pdfMergeSplitJobKind, pendingRun bool) {
	state := &a.pdfMergeSplitTool
	if state.busy {
		return
	}
	mode := state.mode
	var picker func(context.Context, filePickerLabels) (string, error)
	if kind == pdfMergeSplitJobMergeFolder {
		picker = state.mergeDraft.chooseOutputFolder
	} else {
		picker = state.splitDraft.chooseOutputFolder
	}
	if picker == nil {
		picker = chooseOfficeFolderContext
	}
	labels := a.filePickerLabels()
	state.pendingRun = pendingRun
	a.startPDFMergeSplitJob(kind, mode, func(ctx context.Context) pdfMergeSplitJobResult {
		folder, err := picker(ctx, labels)
		return pdfMergeSplitJobResult{kind: kind, folder: folder, err: err}
	})
}

func (a *App) startPDFMergeSplitOperation() {
	state := &a.pdfMergeSplitTool
	if state.busy {
		return
	}
	if state.mode == pdfMergeSplitModeMerge {
		a.startPDFMergeOperation()
		return
	}
	a.startPDFSplitOperation()
}

func (a *App) startPDFMergeOperation() {
	state := &a.pdfMergeSplitTool
	if state.busy {
		return
	}
	state.clearOperationResult()
	draft := &state.mergeDraft
	if len(draft.files) < 2 {
		state.lastError = a.textID("pdf_merge_split.validation.need_two_files")
		return
	}
	name := strings.TrimSpace(draft.outputName.Text())
	if !validPDFUtilityName(name, false) {
		state.lastError = a.textID("pdf_merge_split.validation.invalid_name")
		return
	}
	if !strings.HasSuffix(strings.ToLower(name), ".pdf") {
		name += ".pdf"
	}
	if draft.outputFolder == "" {
		a.startPDFMergeSplitFolderPicker(pdfMergeSplitJobMergeFolder, true)
		return
	}
	sources := make([]string, len(draft.files))
	for index, file := range draft.files {
		sources[index] = file.path
	}
	target := filepath.Join(draft.outputFolder, name)
	state.clearOperationResult()
	runtime := a.ensureDesktopRuntime().applicationRuntime
	runtimeErr := a.ensureDesktopRuntime().applicationRuntimeErr
	a.startPDFMergeSplitJob(pdfMergeSplitJobOperation, pdfMergeSplitModeMerge, func(ctx context.Context) pdfMergeSplitJobResult {
		result := pdfMergeSplitJobResult{kind: pdfMergeSplitJobOperation}
		if runtime == nil {
			result.err = runtimeErr
			if result.err == nil {
				result.err = errors.New("PDF services are unavailable")
			}
			return result
		}
		output, err := runtime.MergePDFUtilityFiles(ctx, sources, target)
		if output.Path != "" {
			result.outputs = []application.PDFUtilityOutput{output}
		}
		result.err = err
		return result
	})
}

func (a *App) startPDFSplitOperation() {
	state := &a.pdfMergeSplitTool
	if state.busy {
		return
	}
	state.clearOperationResult()
	draft := &state.splitDraft
	if draft.source == nil {
		state.lastError = a.textID("pdf_merge_split.validation.need_source")
		return
	}
	prefix := strings.TrimSpace(draft.prefix.Text())
	if !validPDFUtilityName(prefix, true) {
		state.lastError = a.textID("pdf_merge_split.validation.invalid_name")
		return
	}
	var ranges []application.PDFUtilityPageRange
	mode := application.PDFUtilityModeRanges
	if draft.rangeMode == pdfMergeSplitSplitModeRanges {
		parsed, err := application.ParsePDFUtilityPageRanges(draft.rangeInput.Text(), draft.source.pageCount)
		if err != nil {
			state.lastError = a.textID("pdf_merge_split.validation.invalid_ranges")
			return
		}
		ranges = parsed
	} else {
		mode = application.PDFUtilityModeByPage
	}
	if draft.outputFolder == "" {
		a.startPDFMergeSplitFolderPicker(pdfMergeSplitJobSplitFolder, true)
		return
	}
	source, folder := draft.source.path, draft.outputFolder
	state.clearOperationResult()
	runtime := a.ensureDesktopRuntime().applicationRuntime
	runtimeErr := a.ensureDesktopRuntime().applicationRuntimeErr
	a.startPDFMergeSplitJob(pdfMergeSplitJobOperation, pdfMergeSplitModeSplit, func(ctx context.Context) pdfMergeSplitJobResult {
		result := pdfMergeSplitJobResult{kind: pdfMergeSplitJobOperation}
		if runtime == nil {
			result.err = runtimeErr
			if result.err == nil {
				result.err = errors.New("PDF services are unavailable")
			}
			return result
		}
		result.outputs, result.err = runtime.SplitPDFUtilityFile(ctx, source, folder, prefix, mode, ranges)
		return result
	})
}

func validPDFUtilityName(value string, allowExtension bool) bool {
	value = strings.TrimSpace(value)
	if value == "" || value == "." || value == ".." || filepath.Base(value) != value || strings.ContainsRune(value, 0) || strings.ContainsAny(value, `/\\<>:"|?*`) || strings.TrimRight(value, " .") != value {
		return false
	}
	deviceName := strings.ToUpper(strings.TrimRight(strings.TrimSuffix(value, filepath.Ext(value)), " ."))
	switch deviceName {
	case "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return false
	}
	if !allowExtension {
		extension := filepath.Ext(value)
		if extension != "" && !strings.EqualFold(extension, ".pdf") {
			return false
		}
		if strings.EqualFold(extension, ".pdf") && len(value) == len(extension) {
			return false
		}
	}
	return true
}

func (a *App) startPDFMergeSplitJob(kind pdfMergeSplitJobKind, mode pdfMergeSplitMode, work func(context.Context) pdfMergeSplitJobResult) {
	state := &a.pdfMergeSplitTool
	if state.busy || work == nil {
		return
	}
	state.generation++
	generation := state.generation
	ctx, cancel := context.WithCancel(context.Background())
	state.busy, state.cancel = true, cancel
	window := a.window
	started := a.launchManagedBackground(func() {
		defer cancel()
		result := work(ctx)
		result.generation, result.mode, result.kind = generation, mode, kind
		select {
		case state.jobResults <- result:
			if window != nil {
				window.Invalidate()
			}
		case <-a.shutdown:
		}
	})
	if !started {
		cancel()
		state.busy, state.cancel = false, nil
		state.lastError = a.textID("pdf_merge_split.status.error")
	}
}

func (a *App) processPDFMergeSplitToolJobResults() {
	state := &a.pdfMergeSplitTool
	for {
		select {
		case result := <-state.jobResults:
			a.applyPDFMergeSplitToolJobResult(result)
		default:
			return
		}
	}
}

func (a *App) applyPDFMergeSplitToolJobResult(result pdfMergeSplitJobResult) {
	state := &a.pdfMergeSplitTool
	if result.generation != state.generation || result.mode != state.mode {
		return
	}
	state.busy, state.cancel = false, nil
	if result.err != nil {
		if errors.Is(result.err, context.Canceled) {
			state.cancelled = true
			state.lastError = ""
		} else {
			state.lastError = result.err.Error()
		}
	}
	switch result.kind {
	case pdfMergeSplitJobMergeFile:
		if result.err == nil {
			if len(result.files) > 0 {
				for _, file := range result.files {
					a.appendPDFMergeSplitFile(file, true)
				}
			} else if result.file.Path != "" {
				a.appendPDFMergeSplitFile(result.file, true)
			}
		}
	case pdfMergeSplitJobSplitSource:
		if result.err == nil && result.file.Path != "" {
			a.setPDFMergeSplitSource(result.file)
		}
	case pdfMergeSplitJobMergeFolder:
		if result.err == nil && result.folder != "" {
			state.mergeDraft.outputFolder = result.folder
			if state.pendingRun {
				state.pendingRun = false
				a.startPDFMergeOperation()
			}
		} else {
			state.pendingRun = false
		}
	case pdfMergeSplitJobSplitFolder:
		if result.err == nil && result.folder != "" {
			state.splitDraft.outputFolder = result.folder
			if state.pendingRun {
				state.pendingRun = false
				a.startPDFSplitOperation()
			}
		} else {
			state.pendingRun = false
		}
	case pdfMergeSplitJobOperation:
		state.lastOutputs = make([]string, len(result.outputs))
		for index, output := range result.outputs {
			state.lastOutputs[index] = output.Path
		}
	}
	if result.err == nil {
		state.lastError = ""
		state.cancelled = false
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) cancelPDFMergeSplitJobForExit() {
	state := &a.pdfMergeSplitTool
	state.generation++
	state.pendingRun = false
	if state.cancel != nil {
		state.cancel()
	}
	state.cancel, state.busy = nil, false
	state.clearOperationResult()
}

func (a *App) cancelPDFMergeSplitJobForShutdown() {
	a.cancelPDFMergeSplitJobForExit()
}
