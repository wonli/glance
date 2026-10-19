// Package windowmove provides a narrow Windows native-drag redraw workaround.
// Gio remains an unmodified upstream dependency.
package windowmove

import (
	"os"
	"strings"
)

// Enabled defaults to true. Set GLANCE_WINDOWS_MOVE_REDRAW=0 to restore Gio's
// original message handling. Read once per window when attaching the bridge.
func Enabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("GLANCE_WINDOWS_MOVE_REDRAW"))) {
	case "0", "false", "off":
		return false
	default:
		return true
	}
}

type geometry struct {
	width, height int32
	dpi           uint32
	normal, valid bool
}

func deferMovePaint(dragging, resizing bool, before, after geometry) bool {
	return dragging && !resizing && before.valid && after.valid &&
		before.normal && after.normal && before == after
}

func skipPositionRedraw(flags uint32, dragging bool, before, after geometry) bool {
	const (
		noSize       = 0x0001
		noMove       = 0x0002
		noZOrder     = 0x0004
		frameChanged = 0x0020
		showWindow   = 0x0040
		hideWindow   = 0x0080
	)
	return dragging && before.valid && after.valid && before.normal && after.normal &&
		before == after && flags&(noSize|noZOrder) == noSize|noZOrder &&
		flags&(noMove|frameChanged|showWindow|hideWindow) == 0
}
