package desktopui

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"glance/internal/fileutil"
	"glance/internal/proofhistory"
	"glance/internal/proofreading"
	"glance/internal/workspace"
)

type proofreadingProjectPayload struct {
	ConfirmedCurrent map[int]bool                    `json:"confirmed_current,omitempty"`
	Head             string                          `json:"head,omitempty"`
	Draft            proofreadingPersistedDraft      `json:"draft"`
	Page             int                             `json:"page"`
	Offset           int                             `json:"offset"`
	XOffsets         []float64                       `json:"x_offsets,omitempty"`
	Confirmed        map[int]string                  `json:"confirmed,omitempty"`
	Snapshots        map[int]string                  `json:"snapshots,omitempty"`
	Pushed           string                          `json:"pushed,omitempty"`
	OCRJob           *proofreadingOCRJobRecord       `json:"ocr_job,omitempty"`
	OCRBatch         *proofreadingOCRBatchCheckpoint `json:"ocr_batch,omitempty"`
}

func (a *App) proofreadingStore() *workspace.Store {
	if a.desktopRuntime == nil {
		return nil
	}
	a.desktopRuntime.mu.Lock()
	defer a.desktopRuntime.mu.Unlock()
	return a.desktopRuntime.workspaceStore
}
func persistProofreadingProject(store *workspace.Store, p workspace.ProofreadingProject, data proofreadingProjectPayload, aReader io.ReadCloser, bPath string) error {
	if aReader != nil {
		defer aReader.Close()
	}
	if store == nil {
		return nil
	}
	sources := filepath.Join(p.Directory, "sources")
	if err := os.MkdirAll(sources, 0700); err != nil {
		return err
	}
	if _, err := os.Stat(p.SourceA); errors.Is(err, os.ErrNotExist) {
		if aReader == nil {
			return errors.New("reference PDF snapshot is unavailable")
		}
		if err = writeProofreadingSource(p.SourceA, aReader); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if _, err := os.Stat(p.SourceB); errors.Is(err, os.ErrNotExist) {
		f, e := os.Open(bPath)
		if e != nil {
			return e
		}
		err = writeProofreadingSource(p.SourceB, f)
		f.Close()
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	data.ConfirmedCurrent = proofreadingConfirmedCurrent(data, p.ID)
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	p.Payload = payload
	p.CurrentPage = data.Page
	p.UpdatedAt = time.Now().UTC()
	p.ReviewedCount = 0
	if data.Draft.Model != nil {
		p.PageCount = len(data.Draft.Model.Pages)
		for _, page := range data.Draft.Model.Pages {
			if page.Reviewed {
				p.ReviewedCount++
			}
		}
	}

	return store.SaveProofreadingProject(p)
}
func writeProofreadingSource(path string, reader io.Reader) error {
	_, err := fileutil.AtomicWriteReader(path, reader, fileutil.WithAutoCreateDir(true))
	return err
}
func (a *App) proofreadingProjectMetadata() workspace.ProofreadingProject {
	s := a.proofreading
	dir := filepath.Dir(s.draftFilePath)
	return workspace.ProofreadingProject{ID: s.projectDocumentID, Title: s.aDocument.Name, Directory: dir, SourceA: filepath.Join(dir, "sources", "reference.pdf"), SourceAHash: s.aSourceHash, SourceB: filepath.Join(dir, "sources", "manuscript"+filepath.Ext(s.bDocument.Path))}
}
func proofreadingPageResult(s *proofreadingWorkspace, page int, identity proofhistory.Identity) proofhistory.PageResult {
	snap := s.controller.Model.PageSnapshot(proofreadingPageID(page))
	p := proofhistory.PageResult{Next: snap.Next, Version: 1, ProjectID: s.projectDocumentID, SourceHash: s.aSourceHash, Identity: identity, Runs: snap.Runs}
	if len(snap.Pages) > 0 {
		p.Page = snap.Pages[0]
	}
	p.Page.Reviewed = true
	p.Page.Locked = false
	return p
}

func proofreadingPageID(page int) proofreading.PageID { return proofreading.PageID(page + 1) }

func proofreadingConfirmedCurrent(data proofreadingProjectPayload, projectID string) map[int]bool {
	result := map[int]bool{}
	if data.Draft.Model == nil {
		return result
	}
	runs := map[proofreading.PageID][]proofreading.Run{}
	for _, r := range data.Draft.Model.Runs {
		runs[r.Page] = append(runs[r.Page], r)
	}
	for _, page := range data.Draft.Model.Pages {
		index := int(page.ID) - 1
		if hash := data.Confirmed[index]; hash != "" {
			page.Reviewed = true
			page.Locked = false
			current := proofhistory.PageResult{Version: 1, ProjectID: projectID, SourceHash: data.Draft.SourceAHash, Page: page, Runs: runs[page.ID]}
			result[index] = current.ContentHash() == hash
		}
	}
	return result
}
