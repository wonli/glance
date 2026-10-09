package main

import (
	"io"
	"os"

	"glance/internal/desktopui"
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
	return officeqa.DispatchCLI(args, headlessOfficeRenderer{}, stdout, stderr)
}
