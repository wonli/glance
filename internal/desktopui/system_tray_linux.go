//go:build linux

package desktopui

import (
	"errors"
	"fmt"

	"github.com/godbus/dbus/v5"
)

const statusNotifierWatcher = "org.kde.StatusNotifierWatcher"

func trayHostAvailable() error {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return fmt.Errorf("connect to the session D-Bus for the system tray: %w", err)
	}
	defer conn.Close()

	hasWatcher, err := conn.NameHasOwner(statusNotifierWatcher)
	if err != nil {
		return fmt.Errorf("check for StatusNotifierWatcher: %w", err)
	}
	if !hasWatcher {
		return errors.New("no StatusNotifierWatcher is available in this Linux session")
	}
	return nil
}
