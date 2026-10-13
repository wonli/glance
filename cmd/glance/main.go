package main

import (
	"io"
	"os"

	"glance/internal/desktoplaunch"
)

func main() {
	if handled, exitCode := dispatchBeforeDesktop(os.Args[1:], os.Stdout, os.Stderr); handled {
		os.Exit(exitCode)
	}
	path := ""
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	desktoplaunch.RunDesktop(path)
}

func dispatchBeforeDesktop(args []string, stdout, stderr io.Writer) (bool, int) {
	return desktoplaunch.DispatchCLI(args, stdout, stderr)
}
