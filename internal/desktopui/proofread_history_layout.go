package desktopui

import (
	"context"
	"fmt"
	"gioui.org/layout"
	"gioui.org/widget/material"
	"glance/internal/application"
	"glance/internal/proofhistory"
	"glance/internal/proofreading"
)

func (a *App) layoutProofreadingHistory(gtx layout.Context) layout.Dimensions {
	w := a.ensureProofreadingWorkbench()
	return w.list.Layout(gtx, len(w.history)+1, func(g layout.Context, i int) layout.Dimensions {
		if i == 0 {
			if w.diff == "" {
				return material.Body2(a.theme, a.textID("proofread.history.choose")).Layout(g)
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(g, layout.Rigid(material.Body2(a.theme, w.diff).Layout), layout.Rigid(func(g layout.Context) layout.Dimensions {
				return a.proofreadingAction(g, "restore-history", "proofread.history.restore", a.restoreProofreadingHistory)
			}))
		}
		entry := w.history[i-1]
		return layout.Inset{Top: 12, Bottom: 12}.Layout(g, func(g layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(g,
				layout.Flexed(1, material.Body2(a.theme, fmt.Sprintf("%s · %s\n%s", entry.Author, entry.Date, entry.Message)).Layout),
				layout.Rigid(func(g layout.Context) layout.Dimensions {
					return a.proofreadingAction(g, "diff-"+entry.Hash, "proofread.history.view", func() { a.readProofreadingHistory(entry.Hash) })
				}),
			)
		})
	})
}
func (a *App) readProofreadingHistory(commit string) {
	w := a.ensureProofreadingWorkbench()
	p, page := w.historyProject, proofreading.PageID(w.historyPage)
	a.proofreadingWorkbenchJob(func() func() {
		repo := proofhistory.Repository{Directory: p.Directory}
		var value string
		var err error
		if page != 0 {
			var old proofhistory.PageResult
			old, err = repo.ReadPage(context.Background(), commit, page)
			value = old.Text()
			entries, e := repo.List(context.Background(), page)
			if e == nil {
				for i, entry := range entries {
					if entry.Hash == commit {
						before := ""
						if i+1 < len(entries) {
							previous, readErr := repo.ReadPage(context.Background(), entries[i+1].Hash, page)
							if readErr == nil {
								before = previous.Text()
							}
						}
						value = "−\n" + before + "\n\n+\n" + old.Text()
						break
					}
				}
			}
		} else {
			value, err = repo.Diff(context.Background(), commit)
		}
		return func() {
			w.selectedCommit = commit
			if err != nil {
				w.message = err.Error()
			} else {
				w.diff = value
				if w.diff == "" {
					w.diff = a.textID("proofread.history.empty")
				}
			}
		}
	})
}
func (a *App) restoreProofreadingHistory() {
	w := a.ensureProofreadingWorkbench()
	s := a.proofreading
	if s == nil || s.controller == nil || s.projectDocumentID != w.historyProject.ID || w.historyPage == 0 {
		w.message = a.textID("proofread.history.open_page")
		return
	}
	for i := range s.xPageEditors {
		if editorHasActiveComposition(&s.xPageEditors[i]) {
			w.message = a.textID("proofread.history.ime")
			return
		}
	}
	p, page, commit := w.historyProject, proofreading.PageID(w.historyPage), w.selectedCommit

	captured := s.controller.Model.Capture()
	revision := s.controller.Model.Revision()
	taskID, sourceHash := s.projectDocumentID, s.aSourceHash
	a.proofreadingWorkbenchJob(func() func() {
		old, err := (proofhistory.Repository{Directory: p.Directory}).ReadPage(context.Background(), commit, page)
		var model *proofreading.Model
		if err == nil && (old.ProjectID != taskID || old.SourceHash != sourceHash) {
			err = fmt.Errorf("source mismatch")
		}
		if err == nil {
			var candidate proofreading.Snapshot
			candidate, err = application.RestoreProofreadingPage(captured.Snapshot(), old)
			if err == nil {
				model, err = proofreading.Restore(candidate)
			}
		}
		return func() {
			if err != nil {
				w.message = a.textID("proofread.history.conflict")
				return
			}
			if err = s.controller.Model.AcceptRestored(model, revision); err != nil {
				w.message = a.textID("proofread.history.conflict")
				return
			}
			a.proofreadingModelChanged(false)
			w.active = false
			w.historyOpen = false
		}
	})
}
