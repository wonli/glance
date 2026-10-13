package desktopui

import _ "embed"

//go:embed assets/brand/glance-mark.png
var embeddedTrayIcon []byte

func trayIconPNG() []byte { return embeddedTrayIcon }
