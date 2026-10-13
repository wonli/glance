//go:build windows

package desktoplaunch

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"syscall"

	"golang.org/x/sys/windows"
)

const attachParentProcess = ^uintptr(0)

var (
	kernel32                 = windows.NewLazySystemDLL("kernel32.dll")
	attachConsoleProcedure   = kernel32.NewProc("AttachConsole")
	allocateConsoleProcedure = kernel32.NewProc("AllocConsole")
	retainedStandardFiles    []*os.File
	standardFilesMu          sync.Mutex
)

type standardHandles struct {
	stdin  windows.Handle
	stdout windows.Handle
	stderr windows.Handle
	okIn   bool
	okOut  bool
	okErr  bool
}

func PrepareCLIConsole() error {
	inherited := currentStandardHandles()
	_, attachErr := attachParentConsole()
	attached := currentStandardHandles()
	selected := mergeStandardHandles(inherited, attached)

	if !selected.okOut && !selected.okErr {
		if err := allocateConsole(); err != nil {
			refreshed := mergeStandardHandles(inherited, currentStandardHandles())
			if !refreshed.okOut && !refreshed.okErr {
				if attachErr != nil {
					return fmt.Errorf("attach to the parent console (%v) and allocate a console: %w", attachErr, err)
				}
				return fmt.Errorf("allocate a console: %w", err)
			}
			selected = refreshed
		} else {
			selected = mergeStandardHandles(inherited, currentStandardHandles())
		}
	}

	if !selected.okOut && !selected.okErr {
		return errors.New("no usable standard output handle is available")
	}
	if err := applyStandardHandles(selected); err != nil {
		return err
	}
	return nil
}

func currentStandardHandles() standardHandles {
	stdin, okIn := usableStandardHandle(windows.STD_INPUT_HANDLE)
	stdout, okOut := usableStandardHandle(windows.STD_OUTPUT_HANDLE)
	stderr, okErr := usableStandardHandle(windows.STD_ERROR_HANDLE)
	return standardHandles{stdin: stdin, stdout: stdout, stderr: stderr, okIn: okIn, okOut: okOut, okErr: okErr}
}

func usableStandardHandle(which uint32) (windows.Handle, bool) {
	handle, err := windows.GetStdHandle(which)
	if err != nil || handle == 0 || handle == windows.InvalidHandle {
		return 0, false
	}
	typeID, err := windows.GetFileType(handle)
	return handle, err == nil && typeID != windows.FILE_TYPE_UNKNOWN
}

func mergeStandardHandles(preferred, fallback standardHandles) standardHandles {
	if preferred.okIn {
		fallback.stdin, fallback.okIn = preferred.stdin, true
	}
	if preferred.okOut {
		fallback.stdout, fallback.okOut = preferred.stdout, true
	}
	if preferred.okErr {
		fallback.stderr, fallback.okErr = preferred.stderr, true
	}
	return fallback
}

func attachParentConsole() (bool, error) {
	if err := attachConsoleProcedure.Find(); err != nil {
		return false, err
	}
	result, _, callErr := attachConsoleProcedure.Call(attachParentProcess)
	if result != 0 {
		return true, nil
	}
	if errors.Is(callErr, windows.ERROR_ACCESS_DENIED) {
		return true, nil
	}
	if callErr == syscall.Errno(0) || errors.Is(callErr, windows.ERROR_INVALID_HANDLE) {
		return false, nil
	}
	return false, callErr
}

func allocateConsole() error {
	if err := allocateConsoleProcedure.Find(); err != nil {
		return err
	}
	result, _, callErr := allocateConsoleProcedure.Call()
	if result != 0 || errors.Is(callErr, windows.ERROR_ACCESS_DENIED) {
		return nil
	}
	if callErr == syscall.Errno(0) {
		return windows.ERROR_GEN_FAILURE
	}
	return callErr
}

func applyStandardHandles(handles standardHandles) error {
	if handles.okIn {
		if err := windows.SetStdHandle(windows.STD_INPUT_HANDLE, handles.stdin); err != nil {
			return fmt.Errorf("set standard input handle: %w", err)
		}
		if err := refreshStandardFile(&os.Stdin, handles.stdin, "stdin"); err != nil {
			return err
		}
	}
	if handles.okOut {
		if err := windows.SetStdHandle(windows.STD_OUTPUT_HANDLE, handles.stdout); err != nil {
			return fmt.Errorf("set standard output handle: %w", err)
		}
		if err := refreshStandardFile(&os.Stdout, handles.stdout, "stdout"); err != nil {
			return err
		}
	}
	if handles.okErr {
		if err := windows.SetStdHandle(windows.STD_ERROR_HANDLE, handles.stderr); err != nil {
			return fmt.Errorf("set standard error handle: %w", err)
		}
		if err := refreshStandardFile(&os.Stderr, handles.stderr, "stderr"); err != nil {
			return err
		}
	}
	return nil
}

func refreshStandardFile(target **os.File, handle windows.Handle, name string) error {
	if *target != nil && (*target).Fd() == uintptr(handle) {
		return nil
	}
	replacement := os.NewFile(uintptr(handle), name)
	if replacement == nil {
		return fmt.Errorf("create %s file from the attached console handle", name)
	}
	standardFilesMu.Lock()
	if *target != nil {
		retainedStandardFiles = append(retainedStandardFiles, *target)
	}
	*target = replacement
	standardFilesMu.Unlock()
	return nil
}
