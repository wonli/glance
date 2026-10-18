//go:build darwin && !ios && cgo

#import <AppKit/AppKit.h>
#include <stdint.h>
#include <stdlib.h>
#include "_cgo_export.h"

typedef struct {
 uintptr_t key;
 NSView *view;
 id monitor;
} GlancePinchMonitor;

void *glance_installPinch(uintptr_t view, uintptr_t key) {
 GlancePinchMonitor *context = calloc(1, sizeof(GlancePinchMonitor));
 context->key = key;
 context->view = [(NSView *)view retain];
 // Gio hands us a retained public view handle on its window goroutine;
 // all AppKit registration and teardown stays on the native main thread.
 dispatch_async(dispatch_get_main_queue(), ^{
  context->monitor = [[NSEvent addLocalMonitorForEventsMatchingMask:NSEventMaskMagnify handler:^NSEvent *(NSEvent *event) {
   NSView *target = context->view;
   if (event.window != target.window || !target.window) return event;
   NSPoint point = [target convertPoint:event.locationInWindow fromView:nil];
   if (!NSPointInRect(point, target.bounds)) return event;
   CGFloat scale = target.window.backingScaleFactor;
   BOOL begin = (event.phase & NSEventPhaseBegan) != 0;
   BOOL end = (event.phase & (NSEventPhaseEnded | NSEventPhaseCancelled)) != 0;
   glance_onPinch(context->key, point.x * scale, (target.bounds.size.height - point.y) * scale,
                  event.magnification, begin, end);
   return nil;
  }] retain];
 });
 return context;
}

void glance_removePinch(void *monitor) {
 if (!monitor) return;
 GlancePinchMonitor *context = monitor;
 dispatch_async(dispatch_get_main_queue(), ^{
  if (context->monitor) {
   [NSEvent removeMonitor:context->monitor];
   [context->monitor release];
  }
  [context->view release];
  free(context);
 });
}
