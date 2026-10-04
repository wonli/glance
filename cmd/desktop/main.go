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
	done := desktopui.Run(path)
	go func() {
		<-done
		os.Exit(0)
	}()
	app.Main()
}
