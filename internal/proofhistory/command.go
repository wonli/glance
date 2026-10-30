package proofhistory

import (
	"context"
	"os/exec"
)

// Proofreading history commands run in the background and must not open a console.
func newProofhistoryCommand(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	configureProofhistoryCommand(cmd)
	return cmd
}
