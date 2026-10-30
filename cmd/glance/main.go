package main

import (
	"io"
	"os"

	"glance/internal/desktoplaunch"
	"glance/internal/desktopui"
)

func main() {
	args := desktopArgumentsWithoutChromePreview(os.Args[1:])
	if handled, exitCode := dispatchBeforeDesktop(args, os.Stdout, os.Stderr); handled {
		os.Exit(exitCode)
	}
	path, options := parseDesktopArguments(os.Args[1:])
	desktoplaunch.RunDesktop(path, options)
}

func desktopArgumentsWithoutChromePreview(args []string) []string {
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if arg != "--simulate-windows-chrome" {
			filtered = append(filtered, arg)
		}
	}
	return filtered
}

func parseDesktopArguments(args []string) (string, desktopui.DesktopOptions) {
	options := desktopui.DesktopOptions{}
	for _, arg := range args {
		if arg == "--simulate-windows-chrome" {
			options.SimulateWindowsChrome = true
		}
	}
	filtered := desktopArgumentsWithoutChromePreview(args)
	if len(filtered) > 0 {
		return filtered[0], options
	}
	return "", options
}

func dispatchBeforeDesktop(args []string, stdout, stderr io.Writer) (bool, int) {
	return desktoplaunch.DispatchCLI(args, stdout, stderr)
}
