package desktoplaunch

import (
	"glance/internal/diagramrender"
	"io"
	"strings"

	"glance/internal/mcpserver"
	"glance/internal/officeqa"
)

// DispatchCLI handles command-line entrypoints that must run before the desktop
// window is created. It preserves MCP's precedence over Office QA commands.
func DispatchCLI(args []string, stdout, stderr io.Writer) (bool, int) {
	defer diagramrender.CloseDefault()
	if handled, exitCode := mcpserver.DispatchCLI(args, stdout, stderr); handled {
		return true, exitCode
	}
	return officeqa.DispatchCLI(args, headlessOfficeRenderer{}, stdout, stderr)
}

// IsCLIInvocation keeps the no-argument launch and a single document path on
// the GUI path, while attaching a console for explicit CLI commands and flags.
func IsCLIInvocation(args []string) bool {
	if len(args) == 0 {
		return false
	}
	if len(args) > 1 {
		return true
	}
	first := args[0]
	if strings.HasPrefix(first, "-") {
		return true
	}
	switch first {
	case "mcp", "qa", "service", "api", "dal", "help", "completion":
		return true
	default:
		return false
	}
}
