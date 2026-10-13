package desktopui

import (
	"context"
	"errors"
	"runtime"
	"sync"

	"github.com/gogpu/systray"
)

type trayAction uint8

const (
	trayActionShow trayAction = iota + 1
	trayActionQuit
)

type systemTray struct {
	actions chan<- trayAction
	ready   chan struct{}
}

func newSystemTray(actions chan<- trayAction) (*systemTray, error) {
	if actions == nil {
		return nil, errors.New("tray action channel is unavailable")
	}
	if err := trayHostAvailable(); err != nil {
		return nil, err
	}
	return &systemTray{actions: actions, ready: make(chan struct{})}, nil
}

func (t *systemTray) run(ctx context.Context) error {
	if t == nil {
		return errors.New("system tray is unavailable")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	tray := systray.New()
	removed := sync.Once{}
	removeTray := func() { removed.Do(tray.Remove) }
	menu := systray.NewMenu()
	menu.Add("Show Glance", func() { t.send(trayActionShow) })
	menu.AddSeparator()
	menu.Add("Quit Glance", func() { t.send(trayActionQuit) })
	tray.SetAppName("Glance").SetTooltip("Glance").SetIcon(trayIconPNG()).SetMenu(menu)
	tray.OnClick(func() { t.send(trayActionShow) }).Show()
	close(t.ready)

	go func() {
		<-ctx.Done()
		removeTray()
	}()
	err := tray.Run()
	removeTray()
	if ctx.Err() != nil {
		return nil
	}
	if err != nil {
		return err
	}
	return errors.New("system tray event loop exited unexpectedly")
}

func (t *systemTray) send(action trayAction) {
	if t == nil || t.actions == nil {
		return
	}
	select {
	case t.actions <- action:
	default:
	}
}
