package desktopui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
	"glance/internal/i18n"
	"glance/internal/proofhistory"
	"glance/internal/workspace"
)

type proofreadingWorkbenchState struct {
	authMode                                  string
	cancel                                    context.CancelFunc
	active, settings, loaded, busy            bool
	list                                      widget.List
	buttons                                   map[string]*widget.Clickable
	results                                   chan func()
	projects                                  []workspace.ProofreadingProject
	name, email, remote, authUser, token, ssh widget.Editor
	hostKeys, hostURL                         string
	message                                   string
	historyPages                              []proofhistory.PageSummary
	history                                   []proofhistory.PageChange
	historyPageList, historyTimelineList      widget.List
	historyProject                            workspace.ProofreadingProject
	historyPage                               uint64
	historyOpen                               bool
	historyLoading                            bool
	historyMessage                            string
	historyRequest                            uint64
	historyResults                            chan proofreadingHistoryLoadResult
	selectedCommit                            string
	historyTextViews                          map[string]*proofreadingHistoryTextView
}

type proofreadingHistoryLoadResult struct {
	request  uint64
	project  workspace.ProofreadingProject
	pages    []proofhistory.PageSummary
	changes  []proofhistory.PageChange
	page     uint64
	hasPages bool
	err      error
}

func (a *App) ensureProofreadingWorkbench() *proofreadingWorkbenchState {
	if a.proofreadingWorkbench == nil {
		w := &proofreadingWorkbenchState{
			buttons:        map[string]*widget.Clickable{},
			results:        make(chan func(), 4),
			historyResults: make(chan proofreadingHistoryLoadResult, 4),
		}
		w.list.Axis = layout.Vertical
		w.historyPageList.Axis = layout.Vertical
		w.historyTimelineList.Axis = layout.Vertical
		for _, e := range []*widget.Editor{&w.name, &w.email, &w.remote, &w.authUser, &w.token, &w.ssh} {
			e.SingleLine = true
		}
		w.token.Mask = '•'
		a.proofreadingWorkbench = w
	}
	return a.proofreadingWorkbench
}
func (w *proofreadingWorkbenchState) button(id string) *widget.Clickable {
	if w.buttons[id] == nil {
		w.buttons[id] = &widget.Clickable{}
	}
	return w.buttons[id]
}
func (a *App) proofreadingWorkbenchJob(job func() func()) {
	w := a.ensureProofreadingWorkbench()
	if w.busy {
		return
	}
	w.busy = true
	window := a.window
	if !a.launchManagedBackground(func() {
		done := job()
		w.results <- done
		if window != nil {
			window.Invalidate()
		}
	}) {
		w.busy = false
	}
}
func (a *App) processProofreadingWorkbench() {
	w := a.ensureProofreadingWorkbench()
	select {
	case done := <-w.results:
		w.busy = false
		if done != nil {
			done()
		}
	default:
	}
	select {
	case result := <-w.historyResults:
		if result.request != w.historyRequest {
			return
		}
		w.historyLoading = false
		if result.hasPages {
			w.historyPages = result.pages
		}
		w.historyPage = result.page
		w.history = result.changes
		w.historyProject = result.project
		w.remote.SetText(result.project.RemoteURL)
		if result.err != nil {
			w.historyMessage = result.err.Error()
		} else if len(result.pages) == 0 && result.hasPages {
			w.historyMessage = a.textID("proofread.history.empty")
		} else if result.page != 0 && len(result.changes) == 0 {
			w.historyMessage = a.textID("proofread.history.page_empty")
		} else if len(result.changes) > 0 {
			w.selectedCommit = result.changes[0].Entry.Hash
		}
	default:
	}
}
func (a *App) loadProofreadingWorkbench() {
	w := a.ensureProofreadingWorkbench()
	store := a.proofreadingStore()
	if store == nil {
		w.loaded = true
		return
	}
	a.proofreadingWorkbenchJob(func() func() {
		projects, err := store.ListProofreadingProjects()
		identity, _, e := store.Preference("proofreading.identity")
		if err == nil {
			err = e
		}
		auth, _ := loadProofreadingAuth(store)
		var i proofhistory.Identity
		_ = json.Unmarshal([]byte(identity), &i)
		return func() {
			w.loaded = true
			w.projects = projects
			w.name.SetText(i.Name)
			w.email.SetText(i.Email)
			w.authUser.SetText(auth.Username)
			w.ssh.SetText(auth.SSHKey)
			if err != nil {
				w.message = err.Error()
			}
		}
	})
}
func (a *App) saveProofreadingIdentity() {
	w := a.ensureProofreadingWorkbench()
	i := proofhistory.Identity{Name: strings.TrimSpace(w.name.Text()), Email: strings.TrimSpace(w.email.Text())}
	if err := i.Validate(); err != nil {
		w.message = err.Error()
		return
	}
	store := a.proofreadingStore()
	if store == nil {
		return
	}
	a.proofreadingWorkbenchJob(func() func() {
		data, _ := json.Marshal(i)
		err := store.SetPreference("proofreading.identity", string(data))
		if err == nil && a.desktopRuntime != nil {
			a.desktopRuntime.mu.Lock()
			a.desktopRuntime.proofIdentity = i
			a.desktopRuntime.proofIdentityReady = true
			a.desktopRuntime.mu.Unlock()
		}
		return func() {
			if err != nil {
				w.message = err.Error()
			} else {
				w.message = a.textID("proofread.workbench.identity_saved")
			}
		}
	})
}
func (a *App) resumeProofreadingProject(p workspace.ProofreadingProject) {
	if a.proofreading != nil && a.proofreading.projectDocumentID == p.ID {
		a.ensureProofreadingWorkbench().active = false
		return
	}
	w := a.ensureProofreadingWorkbench()
	if r := a.desktopRuntime; r != nil {
		r.mu.Lock()
		owner := r.proofTabOwners[p.ID]
		closing := owner != nil && owner.closing
		r.mu.Unlock()
		if owner != nil {
			if closing {
				w.message = "正在保存并关闭校对稿，请稍后重试"
			} else {
				owner.host.requestProofreadingTabFocus(owner.id, nil)
			}
			return
		}
	}
	store := a.proofreadingStore()
	if store == nil {
		return
	}
	a.proofreadingWorkbenchJob(func() func() {
		full, found, err := store.LoadProofreadingProject(p.ID)
		var payload proofreadingProjectPayload
		if err == nil && found {
			err = json.Unmarshal(full.Payload, &payload)
			if err == nil {
				repo := proofhistory.Repository{Directory: full.Directory}
				payload.Head, _ = repo.Head(context.Background())
				if pushed, found, e := store.Preference("proofreading.pushed." + full.ID); e == nil && found {
					payload.Pushed = pushed
				}
				if payload.Draft.Model != nil {
					payload.Draft.Model.Next = max(payload.Draft.Model.Next, repo.AllocationWatermark())
				}
				if payload.Snapshots == nil {
					payload.Snapshots = map[int]string{}
				}
				for page, hash := range payload.Confirmed {
					old, e := repo.ReadPage(context.Background(), "HEAD", proofreadingPageID(page))
					if e == nil && old.ProjectID == full.ID && old.ContentHash() == hash {
						payload.Snapshots[page] = hash
					}
				}
			}
		}
		full.Payload = nil
		var reader *os.File
		if err == nil && !found {
			err = fmt.Errorf("project not found")
		}
		if err == nil {
			reader, err = os.Open(full.SourceA)
		}
		return func() {
			if err != nil {
				w.message = err.Error()
				return
			}
			err = a.openProofreadingTab(proofreadingWindowRequest{resume: &full, resumePayload: &payload, sourcePath: full.SourceA, sourceName: full.Title, sourcePage: full.CurrentPage, sourceSnapshot: reader})
			if err != nil {
				w.message = err.Error()
			}
		}
	})
}

func (a *App) confirmDeleteProofreadingProject(p workspace.ProofreadingProject) {
	title := p.Title
	if title == "" {
		title = a.textID("proofread.workspace.untitled")
	}
	a.modal.ShowConfirmMessage(
		"proofread.workbench.delete",
		i18n.Message{ID: "proofread.workbench.dialog.delete.title"},
		i18n.Message{ID: "proofread.workbench.dialog.delete.body", Args: i18n.Args{"Title": title}},
		modalDialogDangerConfirmStyle,
		func(action string) {
			a.modal.Hide()
			if action != "delete" {
				return
			}
			a.deleteProofreadingProject(p)
		},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}, Style: modalDialogActionSecondary},
		modalDialogActionSpec{ID: "delete", LabelRef: i18n.Message{ID: "common.action.delete"}, Style: modalDialogActionDanger},
	)
}

func (a *App) deleteProofreadingProject(p workspace.ProofreadingProject) {
	w := a.ensureProofreadingWorkbench()
	if w.busy {
		return
	}
	if r := a.desktopRuntime; r != nil {
		r.mu.Lock()
		owner := r.proofTabOwners[p.ID]
		r.mu.Unlock()
		if owner != nil {
			owner.host.closeProofreadingTab(owner.id)
		}
	}
	if a.proofreading != nil && a.proofreading.projectDocumentID == p.ID {
		a.closeProofreadingDraft()
		a.proofreading = nil
	}
	if w.historyOpen && w.historyProject.ID == p.ID {
		w.historyOpen = false
		w.historyPages = nil
		w.history = nil
		w.historyMessage = ""
	}

	store := a.proofreadingStore()
	dir := p.Directory

	a.proofreadingWorkbenchJob(func() func() {
		var dbErr error
		if store != nil {
			dbErr = store.DeleteProofreadingProject(p.ID)
		}
		var fsErr error
		if dir != "" && dir != "/" && dir != "." {
			_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
				if err == nil && info != nil {
					_ = os.Chmod(path, 0o700)
				}
				return nil
			})
			fsErr = os.RemoveAll(dir)
		}
		return func() {
			err := dbErr
			if err == nil {
				err = fsErr
			}
			if err != nil {
				w.message = err.Error()
				a.setStatusErrorMessage(i18n.Message{ID: "proofread.workbench.error.delete_failed"}, err, notificationToast)
			} else {
				for i, item := range w.projects {
					if item.ID == p.ID {
						w.projects = append(w.projects[:i], w.projects[i+1:]...)
						break
					}
				}
				w.message = a.textID("proofread.workbench.delete_success")
				a.setStatusMessage(i18n.Message{ID: "proofread.workbench.delete_success"}, feedbackSuccess, notificationToast)
			}
		}
	})
}

func (a *App) newProofreadingProject() {
	w := a.ensureProofreadingWorkbench()
	labels := a.filePickerLabels()
	labels.OpenDocument = a.textID("proofread.workbench.choose_pdf")
	labels.AllowedExtensions = []string{"pdf"}
	a.proofreadingWorkbenchJob(func() func() {
		path, err := chooseOfficeFile(labels)
		var reader *os.File
		if err == nil && path != "" {
			reader, err = os.Open(path)
		}
		return func() {
			if err != nil {
				w.message = err.Error()
				return
			}
			if reader == nil {
				return
			}
			if err = a.openProofreadingTab(proofreadingWindowRequest{sourcePath: path, sourceName: filepath.Base(path), sourceSnapshot: reader}); err != nil {
				w.message = err.Error()
			}
		}
	})
}
func (a *App) openProofreadingHistory(p workspace.ProofreadingProject, page uint64) {
	w := a.ensureProofreadingWorkbench()
	w.historyTextViews = nil
	w.historyRequest++
	request := w.historyRequest
	w.historyProject = p
	w.historyPage = page
	w.historyOpen = true
	w.historyLoading = true
	w.historyMessage = ""
	w.historyPages = nil
	w.history = nil
	w.selectedCommit = ""
	w.historyPageList.Position = layout.Position{}
	w.historyTimelineList.Position = layout.Position{}
	w.list.Position = layout.Position{}
	window, shutdown := a.window, a.shutdown
	if !a.launchManagedBackground(func() {
		if store := a.proofreadingStore(); store != nil {
			if metadata, found, err := store.LoadProofreadingProjectMetadata(p.ID); err == nil && found {
				p = metadata
			}
		}
		repo := proofhistory.Repository{Directory: p.Directory}
		pages, err := repo.ListPages(context.Background())
		selectedPage := page
		if err == nil && selectedPage == 0 && len(pages) > 0 {
			selectedPage = uint64(pages[0].Page)
		}
		var changes []proofhistory.PageChange
		if err == nil && selectedPage > 0 {
			changes, err = repo.PageChanges(context.Background(), proofreadingPageID(int(selectedPage)-1))
		}
		result := proofreadingHistoryLoadResult{request: request, project: p, pages: pages, changes: changes, page: selectedPage, hasPages: true, err: err}
		select {
		case w.historyResults <- result:
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
		}
	}) {
		w.historyLoading = false
		w.historyMessage = "无法启动校对历史读取"
	}
}

func (a *App) selectProofreadingHistoryPage(page uint64) {
	w := a.ensureProofreadingWorkbench()
	if page == 0 || page == w.historyPage {
		return
	}
	w.historyTextViews = nil
	w.historyRequest++
	request := w.historyRequest
	w.historyPage = page
	w.historyLoading = true
	w.historyMessage = ""
	w.history = nil
	w.selectedCommit = ""
	w.historyTimelineList.Position = layout.Position{}
	project := w.historyProject
	window, shutdown := a.window, a.shutdown
	if !a.launchManagedBackground(func() {
		changes, err := (proofhistory.Repository{Directory: project.Directory}).PageChanges(context.Background(), proofreadingPageID(int(page)-1))
		result := proofreadingHistoryLoadResult{request: request, project: project, changes: changes, page: page, err: err}
		select {
		case w.historyResults <- result:
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
		}
	}) {
		w.historyLoading = false
		w.historyMessage = a.textID("proofread.history.load_failed")
	}
}

func (a *App) savedProofreadingIdentity() proofhistory.Identity {
	r := a.desktopRuntime
	if r == nil {
		return proofhistory.Identity{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.proofIdentity
}
func (a *App) loadProofreadingSavedIdentity() {
	r := a.desktopRuntime
	store := a.proofreadingStore()
	if r == nil || store == nil {
		return
	}
	r.mu.Lock()
	ready := r.proofIdentityReady
	r.mu.Unlock()
	if ready {
		return
	}
	data, _, err := store.Preference("proofreading.identity")
	var identity proofhistory.Identity
	if err == nil {
		_ = json.Unmarshal([]byte(data), &identity)
	}
	r.mu.Lock()
	if !r.proofIdentityReady {
		r.proofIdentity = identity
		r.proofIdentityReady = true
	}
	r.mu.Unlock()
}

func (a *App) processProofreadingNavigation(g layout.Context) {
	w := a.ensureProofreadingWorkbench()
	if a.proofreading != nil && w.button("manuscript-tab").Clicked(g) {
		w.active = false
		g.Execute(op.InvalidateCmd{})
	}
	if a.proofreading != nil && w.button("workbench-tab").Clicked(g) {
		w.active = true
		w.settings = false
		w.historyOpen = false
		w.loaded = false
		w.list.Position = layout.Position{}
		g.Execute(op.InvalidateCmd{})
	}
}
