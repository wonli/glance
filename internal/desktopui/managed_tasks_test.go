package desktopui

import (
	"sync"
	"testing"
	"time"
)

func TestAppShutdownWaitsForManagedBackgroundTasks(t *testing.T) {
	app := &App{shutdown: make(chan struct{})}
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	var releaseOnce sync.Once
	releaseTask := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseTask)

	if !app.launchManagedBackground(func() {
		close(started)
		<-release
		close(finished)
	}) {
		t.Fatal("active App rejected a managed task")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("managed task did not start")
	}

	close(app.shutdown)
	waitDone := make(chan struct{})
	go func() {
		app.stopAndWaitManagedBackground()
		close(waitDone)
	}()
	if app.launchManagedBackground(func() {}) {
		t.Fatal("App accepted a managed task after shutdown began")
	}
	select {
	case <-waitDone:
		t.Fatal("App shutdown returned while a managed task was still active")
	default:
	}

	releaseTask()
	select {
	case <-waitDone:
	case <-time.After(time.Second):
		t.Fatal("App shutdown did not finish after managed work exited")
	}
	select {
	case <-finished:
	default:
		t.Fatal("managed task was not finished when shutdown returned")
	}
}
