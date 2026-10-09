package desktopui

import (
	"fmt"
	"time"

	"glance/internal/workspace"
)

func (r *desktopRuntime) addWordSnapshot(path string, source workspace.WordVersionSource, snapshot []byte) error {
	if r == nil {
		return fmt.Errorf("工作区数据库不可用")
	}
	pathKey := canonicalSessionPath(path)
	if pathKey == "" {
		return fmt.Errorf("Word 文档没有可用于版本历史的路径")
	}
	r.workspaceDBMu.Lock()
	defer r.workspaceDBMu.Unlock()
	if r.workspaceStore == nil {
		return fmt.Errorf("工作区数据库不可用")
	}
	_, _, err := r.workspaceStore.AddWordVersion(workspace.WordVersionInput{
		PathKey: pathKey, DisplayPath: path, Source: source,
		CapturedAt: time.Now().UTC(), Snapshot: snapshot,
	})
	return err
}

func (r *desktopRuntime) loadWordVersion(pathKey string, versionID int64) (workspace.WordVersion, []byte, error) {
	if r == nil {
		return workspace.WordVersion{}, nil, fmt.Errorf("工作区数据库不可用")
	}
	r.workspaceDBMu.Lock()
	defer r.workspaceDBMu.Unlock()
	if r.workspaceStore == nil {
		return workspace.WordVersion{}, nil, fmt.Errorf("工作区数据库不可用")
	}
	return r.workspaceStore.LoadWordVersion(pathKey, versionID)
}

func (r *desktopRuntime) listWordVersions(pathKey string) ([]workspace.WordVersion, error) {
	if r == nil {
		return nil, fmt.Errorf("工作区数据库不可用")
	}
	r.workspaceDBMu.Lock()
	defer r.workspaceDBMu.Unlock()
	if r.workspaceStore == nil {
		return nil, fmt.Errorf("工作区数据库不可用")
	}
	return r.workspaceStore.ListWordVersions(pathKey)
}

func (r *desktopRuntime) recordOpenedWord(shared *sharedDocumentSession) error {
	if shared == nil || shared.word == nil {
		return nil
	}
	shared.adapterMu.RLock()
	path := shared.path
	if path == "" {
		shared.adapterMu.RUnlock()
		return nil
	}
	snapshot, err := shared.word.Snapshot()
	shared.adapterMu.RUnlock()
	if err != nil {
		return err
	}
	return r.addWordSnapshot(path, workspace.WordVersionOpened, snapshot)
}
