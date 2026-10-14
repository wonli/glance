package main

/*
#cgo darwin LDFLAGS: -framework AppKit
#import <AppKit/AppKit.h>
static void hideWindow() {
	dispatch_async(dispatch_get_main_queue(), ^{
		NSWindow *win = NSApp.keyWindow;
		if (!win) win = NSApp.mainWindow;
		NSLog(@"hiding window: %@", win);
		[win orderOut:nil];
	});
}
*/
import "C"
import (
	"fmt"
	"os"
	"time"

	"gioui.org/app"
	"gioui.org/io/system"
	"gioui.org/unit"
)

func main() {
	logFile, _ := os.Create("scratch.log")
	defer logFile.Close()
	fmt.Fprintln(logFile, "starting...")

	go func() {
		w := new(app.Window)
		w.Option(app.Title("Test"), app.Size(unit.Dp(400), unit.Dp(300)))
		
		go func() {
			time.Sleep(1 * time.Second)
			fmt.Fprintln(logFile, "Calling hideWindow()...")
			C.hideWindow()
			
			time.Sleep(2 * time.Second)
			fmt.Fprintln(logFile, "Attempting ActionRaise...")
			w.Perform(system.ActionRaise)
			fmt.Fprintln(logFile, "ActionRaise completed!")
		}()

		for {
			e := w.Event()
			switch e := e.(type) {
			case app.DestroyEvent:
				fmt.Fprintln(logFile, "DestroyEvent")
				return
			case app.FrameEvent:
				gtx := app.NewContext(nil, e)
				e.Frame(gtx.Ops)
			}
		}
	}()
	app.Main()
}
