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
	history                                   []proofhistory.Entry
	historyProject                            workspace.ProofreadingProject
	historyPage                               uint64
	historyOpen                               bool
	diff                                      string
	selectedCommit                            string
}

func (a *App) ensureProofreadingWorkbench() *proofreadingWorkbenchState {
	if a.proofreadingWorkbench == nil {
		w := &proofreadingWorkbenchState{buttons: map[string]*widget.Clickable{}, results: make(chan func(), 4)}
		w.list.Axis = layout.Vertical
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
				owner.host.requestProofreadingTabFocus(owner.id)
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
	w.historyProject = p
	w.historyPage = page
	w.historyOpen = true
	w.active = true
	w.settings = false
	w.diff = ""
	w.history = nil
	w.list.Position = layout.Position{}
	a.proofreadingWorkbenchJob(func() func() {
		if store := a.proofreadingStore(); store != nil {
			if metadata, found, e := store.LoadProofreadingProjectMetadata(p.ID); e == nil && found {
				p = metadata
			}
		}
		entries, err := (proofhistory.Repository{Directory: p.Directory}).List(context.Background(), proofreadingPageID(int(page)-1))
		return func() {
			w.history = entries
			w.historyProject = p
			w.remote.SetText(p.RemoteURL)
			if err != nil {
				w.message = a.textID("proofread.history.empty")
			}
		}
	})
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
