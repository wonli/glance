//go:build windows

package proofhistory

import (
	"os/exec"
	"syscall"
)

func configureProofhistoryCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}
