package desktopui

// A registry entry identifies one writable manuscript tab across all windows.
// Entries stay registered while their final save is being joined on close.
type proofreadingTabOwner struct {
	host    *App
	id      string
	closing bool
}

func (a *App) registerProofreadingTab(tab *proofreadingTab, projectID string) *proofreadingTabOwner {
	r := a.ensureDesktopRuntime()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.proofTabOwners == nil {
		r.proofTabOwners = map[string]*proofreadingTabOwner{}
	}
	if owner := r.proofTabOwners[projectID]; owner != nil {
		return owner
	}
	owner := &proofreadingTabOwner{host: a, id: tab.id}
	r.proofTabOwners[projectID] = owner
	return owner
}
