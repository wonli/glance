package main

import (
	"os"

	"glance/cmd"
	"glance/internal/desktoplaunch"
)

func main() {
	if handled, exitCode := desktoplaunch.DispatchCLI(os.Args[1:], os.Stdout, os.Stderr); handled {
		os.Exit(exitCode)
	}
	cmd.Execute()
}
