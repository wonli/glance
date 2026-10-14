#import <AppKit/AppKit.h>
#import <dispatch/dispatch.h>
#import <objc/runtime.h>
#include <stdlib.h>
#include <string.h>

#include "system_tray_darwin.h"

enum {
	GlanceTrayActionShow = 1,
	GlanceTrayActionQuit = 2,
};

typedef struct {
	uintptr_t trayID;
	size_t png1xLength;
	size_t png2xLength;
	unsigned char *png1xBytes;
	unsigned char *png2xBytes;
} GlanceTrayCreateContext;

typedef struct {
	uintptr_t trayID;
} GlanceTrayRemoveContext;

@interface GlanceTrayMenuTarget : NSObject {
@public
	uintptr_t trayID;
	NSMenu *menu;
}
- (void)statusItemClicked:(id)sender;
- (void)showContextMenuForView:(NSView *)view event:(NSEvent *)event;
- (void)showGlance:(id)sender;
- (void)quitGlance:(id)sender;
@end

@implementation GlanceTrayMenuTarget
- (void)statusItemClicked:(id)sender {
	NSEvent *event = NSApp.currentEvent;
	if (event != nil && (event.modifierFlags & NSEventModifierFlagControl) != 0) {
		[self showContextMenuForView:(NSView *)sender event:event];
		return;
	}
	[NSApp activateIgnoringOtherApps:YES];
	goGlanceTrayAction(trayID, GlanceTrayActionShow);
}
- (void)showContextMenuForView:(NSView *)view event:(NSEvent *)event {
	if (menu != nil && event != nil) {
		[NSMenu popUpContextMenu:menu withEvent:event forView:view];
	}
}
- (void)showGlance:(id)sender {
	(void)sender;
	[NSApp activateIgnoringOtherApps:YES];
	goGlanceTrayAction(trayID, GlanceTrayActionShow);
}
- (void)quitGlance:(id)sender {
	(void)sender;
	goGlanceTrayAction(trayID, GlanceTrayActionQuit);
}
@end

@interface GlanceStatusBarButton : NSStatusBarButton
@end

@implementation GlanceStatusBarButton
- (void)rightMouseDown:(NSEvent *)event {
	GlanceTrayMenuTarget *target = (GlanceTrayMenuTarget *)self.target;
	[target showContextMenuForView:self event:event];
}
@end

static uintptr_t activeTrayID;
static NSStatusItem *activeStatusItem;
static GlanceTrayMenuTarget *activeTarget;
static NSMenu *activeMenu;

static void createStatusItem(void *rawContext) {
	GlanceTrayCreateContext *context = (GlanceTrayCreateContext *)rawContext;
	@autoreleasepool {
		if (activeTrayID != 0 || NSApp == nil) {
			goGlanceTraySetupFinished(context->trayID, 0);
			free(context->png1xBytes);
			free(context->png2xBytes);
			free(context);
			return;
		}

		NSData *png1x = [NSData dataWithBytesNoCopy:context->png1xBytes
																 length:context->png1xLength
																 freeWhenDone:YES];
		if (png1x != nil) {
			context->png1xBytes = NULL;
		}
		NSData *png2x = [NSData dataWithBytesNoCopy:context->png2xBytes
																 length:context->png2xLength
																 freeWhenDone:YES];
		if (png2x != nil) {
			context->png2xBytes = NULL;
		}
		if (png1x == nil || png2x == nil) {
			goGlanceTraySetupFinished(context->trayID, 0);
			free(context->png1xBytes);
			free(context->png2xBytes);
			free(context);
			return;
		}
		NSBitmapImageRep *rep1x = [[NSBitmapImageRep alloc] initWithData:png1x];
		NSBitmapImageRep *rep2x = [[NSBitmapImageRep alloc] initWithData:png2x];
		NSImage *image = nil;
		if (rep1x != nil && rep2x != nil) {
			[rep1x setSize:NSMakeSize(22.0, 22.0)];
			[rep2x setSize:NSMakeSize(22.0, 22.0)];
			image = [[NSImage alloc] initWithSize:NSMakeSize(22.0, 22.0)];
			[image addRepresentation:rep1x];
			[image addRepresentation:rep2x];
		}
		[rep1x release];
		[rep2x release];
		NSStatusBar *statusBar = [NSStatusBar systemStatusBar];
		NSStatusItem *statusItem = [statusBar statusItemWithLength:NSVariableStatusItemLength];
		GlanceTrayMenuTarget *target = [[GlanceTrayMenuTarget alloc] init];
		NSMenu *menu = [[NSMenu alloc] initWithTitle:@"Glance"];

		if (image == nil || statusItem == nil || target == nil || menu == nil) {
			if (statusItem != nil) {
				[statusBar removeStatusItem:statusItem];
			}
			[image release];
			[target release];
			[menu release];
			goGlanceTraySetupFinished(context->trayID, 0);
			free(context->png1xBytes);
			free(context->png2xBytes);
			free(context);
			return;
		}
		target->trayID = context->trayID;
		target->menu = menu;

		NSStatusBarButton *button = statusItem.button;
		if (button == nil) {
			[statusBar removeStatusItem:statusItem];
			[image release];
			[target release];
			[menu release];
			goGlanceTraySetupFinished(context->trayID, 0);
			free(context);
			return;
		}
		object_setClass(button, [GlanceStatusBarButton class]);
		[image setSize:NSMakeSize(22.0, 22.0)];
		button.image = image;
		[image setTemplate:YES];
		button.toolTip = @"Glance";
		[image release];

		NSMenuItem *showItem = [[NSMenuItem alloc] initWithTitle:@"Show Glance"
																	 action:@selector(showGlance:)
																 keyEquivalent:@""];
		showItem.target = target;
		[menu addItem:showItem];
		[showItem release];
		[menu addItem:[NSMenuItem separatorItem]];
		NSMenuItem *quitItem = [[NSMenuItem alloc] initWithTitle:@"Quit Glance"
																	 action:@selector(quitGlance:)
																 keyEquivalent:@""];
		quitItem.target = target;
		[menu addItem:quitItem];
		[quitItem release];
		button.target = target;
		button.action = @selector(statusItemClicked:);
		[button sendActionOn:NSEventMaskLeftMouseUp];
		activeMenu = [menu retain];
		[menu release];

		activeTrayID = context->trayID;
		activeStatusItem = [statusItem retain];
		activeTarget = target;
		goGlanceTraySetupFinished(context->trayID, 1);
		free(context);
	}
}

static void removeStatusItem(void *rawContext) {
	GlanceTrayRemoveContext *context = (GlanceTrayRemoveContext *)rawContext;
	@autoreleasepool {
		if (activeTrayID == context->trayID) {
			if (activeStatusItem != nil) {
				activeStatusItem.button.target = nil;
				activeStatusItem.button.action = NULL;
				activeTarget->menu = nil;
				[[NSStatusBar systemStatusBar] removeStatusItem:activeStatusItem];
				[activeStatusItem release];
				activeStatusItem = nil;
			}
			activeTrayID = 0;
			[activeMenu release];
			activeMenu = nil;
			[activeTarget release];
			activeTarget = nil;
		}
		goGlanceTrayRemoved(context->trayID);
	}
	free(context);
}

static void hideActiveWindow(void *rawContext) {
	(void)rawContext;
	@autoreleasepool {
		NSWindow *window = NSApp.keyWindow;
		if (window == nil) {
			window = NSApp.mainWindow;
		}
		[window orderOut:nil];
	}
}

int glance_tray_create(uintptr_t trayID, const void *png1x, size_t png1xLength, const void *png2x, size_t png2xLength) {
	GlanceTrayCreateContext *context = calloc(1, sizeof(*context));
	if (context == NULL) {
		return 0;
	}
	context->trayID = trayID;
	context->png1xLength = png1xLength;
	context->png2xLength = png2xLength;
	context->png1xBytes = malloc(png1xLength);
	context->png2xBytes = malloc(png2xLength);
	if (context->png1xBytes == NULL || context->png2xBytes == NULL) {
		free(context->png1xBytes);
		free(context->png2xBytes);
		free(context);
		return 0;
	}
	memcpy(context->png1xBytes, png1x, png1xLength);
	memcpy(context->png2xBytes, png2x, png2xLength);
	dispatch_async_f(dispatch_get_main_queue(), context, createStatusItem);
	return 1;
}

int glance_tray_remove(uintptr_t trayID) {
	GlanceTrayRemoveContext *context = malloc(sizeof(*context));
	if (context == NULL) {
		return 0;
	}
	context->trayID = trayID;
	dispatch_async_f(dispatch_get_main_queue(), context, removeStatusItem);
	return 1;
}

int glance_tray_hide_active_window(void) {
	if (NSApp == nil) {
		return 0;
	}
	dispatch_async_f(dispatch_get_main_queue(), NULL, hideActiveWindow);
	return 1;
}
