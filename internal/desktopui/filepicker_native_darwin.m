#import <AppKit/AppKit.h>
#import <dispatch/dispatch.h>
#include <stdlib.h>
#include <string.h>

int glance_choose_save_path(const char *title, const char *name, char **path, char **error) {
	__block int status = 0;
	void (^present)(void) = ^{
		@autoreleasepool {
			NSWindow *owner = NSApp.keyWindow ?: NSApp.mainWindow;
			@try {
				NSSavePanel *panel = [NSSavePanel savePanel];
				panel.title = [NSString stringWithUTF8String:title];
				panel.nameFieldStringValue = [NSString stringWithUTF8String:name];
				panel.canCreateDirectories = YES;
				panel.extensionHidden = NO;
				// This panel belongs to Glance, unlike an external osascript dialog.
				[NSApp activateIgnoringOtherApps:YES];
				[panel makeKeyAndOrderFront:nil];
				NSScreen *screen = owner.screen ?: NSScreen.mainScreen;
				if (screen != nil) {
					NSRect visible = screen.visibleFrame;
					NSRect frame = panel.frame;
					// AppKit's screen coordinates increase upward. Keep a slightly
					// elevated center while leaving the panel inside the usable area.
					CGFloat y = NSMidY(visible) - NSHeight(frame) / 2 + NSHeight(visible) * 0.06;
					y = MAX(NSMinY(visible), MIN(y, NSMaxY(visible) - NSHeight(frame)));
					[panel setFrameOrigin:NSMakePoint(
						NSMidX(visible) - NSWidth(frame) / 2,
						y)];
				}
				if ([panel runModal] == NSModalResponseOK) {
					*path = strdup(panel.URL.path.UTF8String);
					status = *path != NULL ? 1 : -1;
					if (status < 0) {
						*error = strdup("Could not read the selected save path");
					}
				}
				[panel orderOut:nil];
			} @catch (NSException *exception) {
				*error = strdup(exception.reason.UTF8String ?: "Could not open the save panel");
				status = -1;
			}
			if (NSApp.active && owner.visible) {
				[owner makeKeyAndOrderFront:nil];
			}
		}
	};
	if ([NSThread isMainThread]) {
		present();
	} else {
		dispatch_sync(dispatch_get_main_queue(), present);
	}
	return status;
}
