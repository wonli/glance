//go:build windows

package desktopinstance

import (
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestActivationServerCloseWhileWaiting(t *testing.T) {
	stop, err := startActivationServer("", make(chan struct{}, 1))
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- stop() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("activation server did not stop while waiting for a connection")
	}
}

func TestActivationServerCloseWithIdleClient(t *testing.T) {
	stop, err := startActivationServer("", make(chan struct{}, 1))
	if err != nil {
		t.Fatal(err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	name, err := windows.UTF16PtrFromString(`\\.\pipe\GlanceActivate-` + compactSID(user.User.Sid.String()))
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	// Wait until the server has accepted the connection and entered its read.
	time.Sleep(100 * time.Millisecond)
	done := make(chan error, 1)
	go func() { done <- stop() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("activation server did not stop while reading an idle client")
	}
}
