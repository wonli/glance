package main

import (
	"log"

	"gioui.org/app"
)

func main() {
	go func() {
		window := new(app.Window)
		window.Option(app.Title("Glance UI Gallery"), app.Size(1040, 760))
		if err := run(window); err != nil {
			log.Printf("UI gallery closed: %v", err)
		}
	}()
	app.Main()
}
