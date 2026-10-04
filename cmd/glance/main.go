package main

import (
	"os"

	"glance/internal/desktopui"

	"gioui.org/app"
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
