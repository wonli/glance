package desktopui

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"glance/internal/application"
)

type shareCopyDocument interface {
	SaveAsWithCommitGuard(path string, guard func() (func(), error)) error
	Close() error
}

func ensureShareCopyExtension(path, extension string) string {
	suffix := "." + extension
	if strings.EqualFold(filepath.Ext(path), suffix) {
		return path
	}
	return path + suffix
}

func saveShareCopySnapshot[T shareCopyDocument](
	path, format string,
	snapshot func() ([]byte, error),
	revalidate func() error,
	openSnapshot func([]byte) (T, error),
	commitGuard func() (func(), error),
) error {
	payload, err := snapshot()
	if err != nil {
		return err
	}
	if err := revalidate(); err != nil {
		return err
	}

	copyDocument, err := openSnapshot(payload)
	if err != nil {
		return fmt.Errorf("校验 %s 副本：%w", format, err)
	}
	saveErr := copyDocument.SaveAsWithCommitGuard(path, commitGuard)
	closeErr := copyDocument.Close()
	if saveErr != nil {
		return saveErr
	}
	return closeErr
}

func (a *App) applyShareCopyResult(state *documentSessionState, path string, err error, format string) {
	if state == nil || state.closed.Load() {
		return
	}
	if err != nil {
		if state == a.documentSessionState {
			a.setStatus("导出共享副本失败：" + err.Error())
		}
		return
	}
	if state == a.documentSessionState {
		a.setStatus("共享 " + format + " 副本已导出：" + filepath.Base(path))
	}
}

func (a *App) startDocumentExportCopy(state *documentSessionState, path string, revision uint64, publish func(string, error)) bool {
	if state == nil || publish == nil {
		return false
	}
	token, ok := state.beginOperation(sessionOperationSnapshot)
	if !ok || token.revision != revision {
		if ok {
			state.finishOperation(token)
		}
		publish(path, application.ErrRevisionConflict)
		return true
	}
	window := a.window
	a.launchManagedBackground(func() {
		result, err := a.executeRibbonApplicationCommand(token, "document.export_copy", "document_file", path, "", map[string]any{"path": path})
		outputPath := path
		if err == nil {
			var output struct {
				Path string `json:"path"`
			}
			if decodeErr := json.Unmarshal(result.Data, &output); decodeErr != nil || output.Path == "" {
				err = application.ErrInvalidCommandResult
			} else {
				outputPath = output.Path
			}
		}
		a.completeSessionOperation(state, token, err, false)
		publish(outputPath, err)
		if window != nil {
			window.Invalidate()
		}
	})
	return true
}
