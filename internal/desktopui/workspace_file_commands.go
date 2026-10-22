package desktopui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"glance/internal/i18n"
	"glance/internal/workspace"
)

type workspaceWriteJob struct {
	documentID, format string
	tabID              string
	state              *documentSessionState
	viewState          []byte
	homeFolderSnapshot *workspace.HomeFolderSnapshot
	homeFolderRevision uint64
	homeFolderResults  chan homeFolderWriteResult
	barrier            chan struct{}
	recents            []workspace.Recent
	homeRecentResults  chan homeRecentWriteResult
	homeRecentRevision uint64
	favoritePath       string
	favoriteValue      bool
	saveFavorite       bool
	savePreferences    bool
}

type fileCommandKind uint8

const (
	commandOpenFile fileCommandKind = iota + 1
	commandOpenProofreadingSource
	commandOpenFolder
	commandRegisterHomeFolder
	commandSaveAs
	commandExportPDF
	commandShareCopy
	commandRevealFile
)

type fileCommandResult struct {
	kind    fileCommandKind
	path    string
	recents []workspace.Recent
	err     error
}

func (a *App) startOpenFilePicker() {
	a.startFileCommand(commandOpenFile, "")
}

func (a *App) startProofreadingSourcePicker() {
	a.startFileCommand(commandOpenProofreadingSource, "")
}

func (a *App) startFolderPicker() {
	a.startFileCommand(commandOpenFolder, "")
}

func (a *App) startHomeFolderDirectoryPicker() {
	a.startFileCommand(commandRegisterHomeFolder, "")
}

func (a *App) startSaveAsPicker(format string) {
	a.startFileCommand(commandSaveAs, format)
}

func (a *App) startWordPDFSavePicker(state *documentSessionState, key wordLayoutKey, orientation string) {
	if state == nil || state.word == nil || a.fileCommandRunning {
		return
	}
	a.wordPDFPickerRequest = &wordPDFPickerRequest{state: state, key: key, orientation: orientation}
	a.startFileCommand(commandExportPDF, "pdf")
}

// setWordFileCommandRunner is a narrow injection seam for exercising the Word
// ribbon's file commands without opening a native platform picker.
func (a *App) setWordFileCommandRunner(runner func(fileCommandKind, string) fileCommandResult) {
	a.fileCommandRunner = runner
}

func (a *App) startFileCommand(kind fileCommandKind, format string) {
	if a.fileCommandRunning {
		return
	}
	a.fileCommandRunning = true
	if message := fileCommandStartMessage(kind, format); message.ID != "" {
		a.setStatusMessage(message, feedbackInfo, notificationSuppressed)
	}
	runner := a.fileCommandRunner
	pickerLabels := a.filePickerLabels()
	if kind == commandOpenProofreadingSource {
		pickerLabels.OpenDocument = a.textID("pdf.proofread.choose_source")
	}
	a.launchManagedBackground(func() {
		result := fileCommandResult{kind: kind}
		if runner != nil {
			result = runner(kind, format)
			result.kind = kind
			if kind == commandRevealFile && result.path == "" {
				result.path = format
			}
		} else {
			switch kind {
			case commandOpenFile, commandOpenProofreadingSource:
				result.path, result.err = chooseOfficeFile(pickerLabels)
			case commandOpenFolder:
				result.path, result.err = chooseOfficeFolder(pickerLabels)
				if result.err == nil && result.path != "" {
					result.recents, result.err = scanOfficeFolder(result.path)
				}
			case commandRegisterHomeFolder:
				result.path, result.err = chooseOfficeFolder(pickerLabels)
				if result.err == nil && result.path != "" {
					result.path, result.err = validateHomeFolderDirectory(result.path)
				}
			case commandRevealFile:
				result.path = format
				result.err = revealHomeRecentFile(format)
			case commandSaveAs, commandExportPDF, commandShareCopy:
				result.path, result.err = chooseSavePath(format, pickerLabels)
			}
		}
		select {
		case a.fileCommandResults <- result:
			if a.window != nil {
				a.window.Invalidate()
			}
		case <-a.shutdown:
		}
	})
}

func fileCommandStartMessage(kind fileCommandKind, format string) i18n.Message {
	switch kind {
	case commandOpenFile:
		return i18n.Message{ID: "file.command.choose_open_file"}
	case commandOpenProofreadingSource:
		return i18n.Message{ID: "pdf.proofread.choose_source"}
	case commandOpenFolder:
		return i18n.Message{ID: "file.command.choose_open_folder"}
	case commandRegisterHomeFolder:
		return i18n.Message{ID: "file.command.choose_register_folder"}
	case commandSaveAs:
		return i18n.Message{ID: "file.command.choose_save_as"}
	case commandExportPDF:
		return i18n.Message{ID: "file.command.choose_pdf_export"}
	case commandShareCopy:
		if format == "xlsx" {
			return i18n.Message{ID: "file.command.choose_shared_xlsx"}
		}
		return i18n.Message{ID: "file.command.choose_shared_docx"}
	case commandRevealFile:
		return i18n.Message{ID: "file.command.reveal_file"}
	default:
		return i18n.Message{}
	}
}

func fileCommandFailureMessage(kind fileCommandKind) i18n.Message {
	switch kind {
	case commandOpenFile, commandOpenProofreadingSource:
		return i18n.Message{ID: "file.command.open_file_failed"}
	case commandOpenFolder:
		return i18n.Message{ID: "file.command.open_folder_failed"}
	case commandRegisterHomeFolder:
		return i18n.Message{ID: "home.folder.error.register_failed"}
	case commandSaveAs:
		return i18n.Message{ID: "file.command.save_as_failed"}
	case commandExportPDF:
		return i18n.Message{ID: "file.command.pdf_failed"}
	case commandShareCopy:
		return i18n.Message{ID: "file.command.share_failed"}
	case commandRevealFile:
		return i18n.Message{ID: "file.command.reveal_failed"}
	default:
		return i18n.Message{ID: "common.feedback.operation_failed"}
	}
}

func validateHomeFolderDirectory(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	canonical := canonicalDocumentPath(path)
	info, err := os.Stat(canonical)
	if err != nil {
		return "", fmt.Errorf("无法访问所选文件夹：%w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("所选路径不是文件夹：%s", canonical)
	}
	return canonical, nil
}

func scanOfficeFolder(path string) ([]workspace.Recent, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	var recents []workspace.Recent
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".pdf" && ext != ".xlsx" && ext != ".xls" && ext != ".doc" && ext != ".docx" && ext != ".pptx" && ext != ".json" && ext != ".md" && ext != ".markdown" && ext != ".csv" {
			continue
		}
		recents = append(recents, workspace.Recent{Path: filepath.Join(path, entry.Name()), Kind: strings.TrimPrefix(ext, ".")})
	}
	return recents, nil
}

func (a *App) persistRecentsAsync() {
	a.ensureDesktopRuntime()
	if a.collection == nil {
		return
	}
	recents := a.collection.Recents()
	if a.workspaceWriteJobs != nil {
		a.workspaceWriteJobs <- workspaceWriteJob{recents: recents}
		return
	}
	if a.workspaceStore != nil {
		if err := a.workspaceStore.SaveRecents(recents); err != nil {
			a.setStatusErrorMessage(i18n.Message{ID: "workspace.error.save_recent_files"}, err, notificationToast)
		}
	}
}
