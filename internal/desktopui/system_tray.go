//go:build !darwin

package desktopui

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"

	"github.com/gogpu/systray"
)

type systemTray struct {
	queue systemTrayActionQueue
	ready chan struct{}
}

func trayStartupDeferredUntilWindowFrame() bool { return true }

func newSystemTray(actions chan trayAction) (*systemTray, error) {
	if actions == nil {
		return nil, errors.New("tray action channel is unavailable")
	}
	if err := trayHostAvailable(); err != nil {
		return nil, err
	}
	return &systemTray{queue: systemTrayActionQueue{actions: actions}, ready: make(chan struct{})}, nil
}

func (t *systemTray) run(ctx context.Context) error {
	if t == nil {
		return errors.New("system tray is unavailable")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	tray, err := systray.NewChecked()
	if err != nil {
		if tray != nil {
			tray.Remove()
		}
		return fmt.Errorf("create native system tray: %w", err)
	}
	removed := sync.Once{}
	removeTray := func() { removed.Do(tray.Remove) }
	menu := systray.NewMenu()
	menu.Add("Show Glance", func() { t.queue.send(trayActionShow) })
	menu.AddSeparator()
	menu.Add("Quit Glance", func() { t.queue.send(trayActionQuit) })
	tray.SetAppName("Glance")
	if err := tray.SetTooltipChecked("Glance"); err != nil {
		removeTray()
		return fmt.Errorf("set system tray tooltip: %w", err)
	}
	if err := tray.SetIconChecked(trayIconPNG()); err != nil {
		removeTray()
		return fmt.Errorf("set system tray icon: %w", err)
	}
	if err := tray.SetMenuChecked(menu); err != nil {
		removeTray()
		return fmt.Errorf("set system tray menu: %w", err)
	}
	tray.OnClick(func() { t.queue.send(trayActionShow) })
	if err := tray.ShowChecked(); err != nil {
		removeTray()
		return fmt.Errorf("show system tray icon: %w", err)
	}
	close(t.ready)

	go func() {
		<-ctx.Done()
		removeTray()
	}()
	err = tray.Run()
	removeTray()
	if ctx.Err() != nil {
		return nil
	}
	if err != nil {
		return err
	}
	return errors.New("system tray event loop exited unexpectedly")
}
