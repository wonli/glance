package desktopui

import (
	"fmt"
	"glance/internal/workspace"
	"os"
	"path/filepath"
	"strings"
)

type workspaceWriteJob struct {
	documentID, format string
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
	switch kind {
	case commandOpenFile:
		a.setStatus("选择要打开的文件…")
	case commandOpenFolder:
		a.setStatus("选择要浏览的文件夹…")
	case commandRegisterHomeFolder:
		a.setStatus("选择要加入文件夹导航的本地目录…")
	case commandSaveAs:
		a.setStatus("选择另存为位置…")
	case commandExportPDF:
		a.setStatus("选择 PDF 保存位置…")
	case commandShareCopy:
		if format == "xlsx" {
			a.setStatus("选择共享 XLSX 副本保存位置…")
		} else {
			a.setStatus("选择共享 DOCX 副本保存位置…")
		}
	case commandRevealFile:
		a.setStatus("正在打开文件夹…")
	}
	runner := a.fileCommandRunner
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
			case commandOpenFile:
				result.path, result.err = chooseOfficeFile()
			case commandOpenFolder:
				result.path, result.err = chooseOfficeFolder()
				if result.err == nil && result.path != "" {
					result.recents, result.err = scanOfficeFolder(result.path)
				}
			case commandRegisterHomeFolder:
				result.path, result.err = chooseOfficeFolder()
				if result.err == nil && result.path != "" {
					result.path, result.err = validateHomeFolderDirectory(result.path)
				}
			case commandRevealFile:
				result.path = format
				result.err = revealHomeRecentFile(format)
			case commandSaveAs, commandExportPDF, commandShareCopy:
				result.path, result.err = chooseSavePath(format)
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
		if ext != ".pdf" && ext != ".xlsx" && ext != ".xls" && ext != ".docx" && ext != ".pptx" && ext != ".json" && ext != ".md" && ext != ".markdown" {
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
			a.setStatus("工作区保存失败：" + err.Error())
		}
	}
}
