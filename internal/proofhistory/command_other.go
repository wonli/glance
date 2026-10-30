//go:build !windows

package proofhistory

import "os/exec"

func configureProofhistoryCommand(*exec.Cmd) {}
