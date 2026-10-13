package main

import (
	"fmt"
	"os"

	"glance/cmd"
	"glance/internal/desktoplaunch"
)

func main() {
	if desktoplaunch.IsCLIInvocation(os.Args[1:]) {
		if err := desktoplaunch.PrepareCLIConsole(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if handled, exitCode := desktoplaunch.DispatchCLI(os.Args[1:], os.Stdout, os.Stderr); handled {
		os.Exit(exitCode)
	}
	cmd.Execute()
}
