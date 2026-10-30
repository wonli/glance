//go:build windows

package proofhistory

import (
	"context"
	"testing"
)

func TestProofhistoryBackgroundCommandDoesNotCreateConsole(t *testing.T) {
	cmd := newProofhistoryCommand(context.Background(), "git", "status")
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.HideWindow || cmd.SysProcAttr.CreationFlags&0x08000000 == 0 {
		t.Fatalf("background command must not create a console: %+v", cmd.SysProcAttr)
	}
}
