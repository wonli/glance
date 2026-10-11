package main

import (
	"io"
	"os"

	"glance/internal/desktopui"
	"glance/internal/mcpserver"
	"glance/internal/officeqa"

	"gioui.org/app"
)

func main() {
	if handled, exitCode := dispatchBeforeDesktop(os.Args[1:], os.Stdout, os.Stderr); handled {
		os.Exit(exitCode)
	}
	path := ""
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	done := desktopui.Run(path)
	go func() {
		<-done
		os.Exit(0)
	}()
	app.Main()
}

func dispatchBeforeDesktop(args []string, stdout, stderr io.Writer) (bool, int) {
	if handled, exitCode := mcpserver.DispatchCLI(args, stdout, stderr); handled {
		return true, exitCode
	}
	return officeqa.DispatchCLI(args, headlessOfficeRenderer{}, stdout, stderr)
}
