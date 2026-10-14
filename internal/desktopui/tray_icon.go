package desktopui

import _ "embed"

//go:embed assets/brand/glance-mark.png
var embeddedTrayIcon []byte

//go:embed assets/brand/glance-tray-macos-22.png
var embeddedMacTrayIcon22 []byte

//go:embed assets/brand/glance-tray-macos-44.png
var embeddedMacTrayIcon44 []byte

func trayIconPNG() []byte { return embeddedTrayIcon }
