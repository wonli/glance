package desktopui

// launchManagedBackground starts work whose lifetime belongs to one App
// window. The mutex makes task registration atomic with shutdown gating, so a
// task cannot be added after stopAndWaitManagedBackground begins waiting.
func (a *App) launchManagedBackground(task func()) bool {
	if a == nil || task == nil {
		return false
	}
	a.managedBackgroundMu.Lock()
	if a.managedBackgroundClosed || isAppShuttingDown(a.shutdown) {
		a.managedBackgroundClosed = true
		a.managedBackgroundMu.Unlock()
		return false
	}
	a.managedBackgroundTasks.Add(1)
	a.managedBackgroundMu.Unlock()
	go func() {
		defer a.managedBackgroundTasks.Done()
		task()
	}()
	return true
}

func isAppShuttingDown(shutdown <-chan struct{}) bool {
	select {
	case <-shutdown:
		return true
	default:
		return false
	}
}

func (a *App) stopAndWaitManagedBackground() {
	if a == nil {
		return
	}
	a.managedBackgroundMu.Lock()
	a.managedBackgroundClosed = true
	a.managedBackgroundMu.Unlock()
	a.managedBackgroundTasks.Wait()
}
