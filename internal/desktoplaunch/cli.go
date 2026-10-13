package desktoplaunch

import (
	"io"

	"glance/internal/mcpserver"
	"glance/internal/officeqa"
)

// DispatchCLI handles command-line entrypoints that must run before the desktop
// window is created. It preserves MCP's precedence over Office QA commands.
func DispatchCLI(args []string, stdout, stderr io.Writer) (bool, int) {
	if handled, exitCode := mcpserver.DispatchCLI(args, stdout, stderr); handled {
		return true, exitCode
	}
	return officeqa.DispatchCLI(args, headlessOfficeRenderer{}, stdout, stderr)
}
