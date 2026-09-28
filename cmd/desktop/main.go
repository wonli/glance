package main

import (
	"os"

	"gioui.org/app"
	"glance/internal/desktopui"
)

func main() {
	path := ""
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	go desktopui.Run(path)
	app.Main()
}
