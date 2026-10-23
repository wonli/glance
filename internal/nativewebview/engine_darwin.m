//go:build darwin && cgo

#import <Foundation/Foundation.h>
#import <AppKit/AppKit.h>
#import <WebKit/WebKit.h>
#import <objc/message.h>
#include <stdlib.h>
#include <stdint.h>
#include <string.h>

@interface OffscreenWebEngine : NSObject <WKNavigationDelegate>
@property (nonatomic, strong) WKWebView *webView;
@property (nonatomic, assign) BOOL isLoaded;
@property (nonatomic, copy) void (^onLoaded)(void);
@end

@implementation OffscreenWebEngine

- (void)dealloc {
    self.webView.navigationDelegate = nil;
    self.webView = nil;
    self.onLoaded = nil;
    [super dealloc];
}

- (instancetype)initWithHTML:(NSString *)html completion:(void(^)(void))completion {
    self = [super init];
    if (self) {
        self.onLoaded = completion;

        WKWebViewConfiguration *config = [[WKWebViewConfiguration alloc] init];
        config.preferences.javaScriptCanOpenWindowsAutomatically = NO;
        config.websiteDataStore = [WKWebsiteDataStore nonPersistentDataStore];
        WKWebView *view = [[WKWebView alloc] initWithFrame:NSMakeRect(0, 0, 1600, 1600) configuration:config];
        self.webView = view;
        [view release];
        [config release];
        self.webView.navigationDelegate = self;

        [self.webView loadHTMLString:html baseURL:nil];
        // A cancelled navigation or terminated WebContent process need not
        // deliver a navigation failure. Always finish the pending load.
        dispatch_after(dispatch_time(DISPATCH_TIME_NOW, 8 * NSEC_PER_SEC), dispatch_get_main_queue(), ^{
            if (self.onLoaded) {
                self.onLoaded();
                self.onLoaded = nil;
            }
        });
    }
    return self;
}

- (void)webView:(WKWebView *)webView didFinishNavigation:(WKNavigation *)navigation {
    self.isLoaded = YES;
    if (self.onLoaded) {
        self.onLoaded();
        self.onLoaded = nil;
    }
}

- (void)webView:(WKWebView *)webView decidePolicyForNavigationAction:(WKNavigationAction *)navigationAction decisionHandler:(void (^)(WKNavigationActionPolicy))decisionHandler {
    NSURL *url = navigationAction.request.URL;
    // about:blank is an opaque URL: NSURL.path is nil, not "blank".
    BOOL isCurrentDocument = [url.absoluteString.lowercaseString isEqualToString:@"about:blank"];
    BOOL isMainFrame = navigationAction.targetFrame && navigationAction.targetFrame.mainFrame;
    decisionHandler(isCurrentDocument && isMainFrame ? WKNavigationActionPolicyAllow : WKNavigationActionPolicyCancel);
}

- (void)webView:(WKWebView *)webView didFailNavigation:(WKNavigation *)navigation withError:(NSError *)error {
    if (self.onLoaded) {
        self.onLoaded();
        self.onLoaded = nil;
    }
}

- (void)webView:(WKWebView *)webView didFailProvisionalNavigation:(WKNavigation *)navigation withError:(NSError *)error {
    if (self.onLoaded) {
        self.onLoaded();
        self.onLoaded = nil;
    }
}

@end

static OffscreenWebEngine *g_sharedEngine = nil;
static NSString *g_currentHTML = nil;

int glance_nativeweb_setup(const char *html_cstr) {
    @autoreleasepool {
    NSString *newHTML = [NSString stringWithUTF8String:html_cstr];
    if (g_sharedEngine != nil && g_sharedEngine.isLoaded && [g_currentHTML isEqualToString:newHTML]) {
        return 1;
    }

    __block int success = 0;
    dispatch_semaphore_t sem = dispatch_semaphore_create(0);

    dispatch_async(dispatch_get_main_queue(), ^{
        @autoreleasepool {
            [g_currentHTML release];
            g_currentHTML = [newHTML copy];
            [g_sharedEngine release];
            g_sharedEngine = [[OffscreenWebEngine alloc] initWithHTML:newHTML completion:^{
                success = (g_sharedEngine && g_sharedEngine.isLoaded) ? 1 : 0;
                dispatch_semaphore_signal(sem);
            }];
        }
    });

    dispatch_semaphore_wait(sem, DISPATCH_TIME_FOREVER);
    return success;
    }
}

int glance_nativeweb_eval_and_snapshot(
    const char *script_cstr,
    const char *args_json_cstr,
    int density,
    char **out_data,
    int *out_w,
    int *out_h,
    void **out_png,
    size_t *out_len,
    char **out_err
) {
    @autoreleasepool {
    if (!g_sharedEngine || !g_sharedEngine.isLoaded) {
        if (out_err) {
            *out_err = strdup("native WebKit engine is not initialized");
        }
        return 0;
    }

    __block int success = 0;
    dispatch_semaphore_t sem = dispatch_semaphore_create(0);
    NSString *rawScript = [NSString stringWithUTF8String:script_cstr];
    NSString *argsJSONStr = args_json_cstr ? [NSString stringWithUTF8String:args_json_cstr] : @"{}";

    // Ensure async function returns result
    NSString *scriptStr = rawScript;
    NSString *trimmed = [rawScript stringByTrimmingCharactersInSet:[NSCharacterSet whitespaceAndNewlineCharacterSet]];
    if ([trimmed hasPrefix:@"(async"] || [trimmed hasPrefix:@"(function"]) {
        scriptStr = [NSString stringWithFormat:@"return await %@", rawScript];
    }

    dispatch_async(dispatch_get_main_queue(), ^{
        @autoreleasepool {
            NSError *jsonErr = nil;
            NSData *argsData = [argsJSONStr dataUsingEncoding:NSUTF8StringEncoding];
            NSDictionary *argsDict = [NSJSONSerialization JSONObjectWithData:argsData options:0 error:&jsonErr];
            if (![argsDict isKindOfClass:[NSDictionary class]] || jsonErr) {
                argsDict = @{};
            }

            [g_sharedEngine.webView callAsyncJavaScript:scriptStr arguments:argsDict inFrame:nil inContentWorld:[WKContentWorld pageWorld] completionHandler:^(id result, NSError *error) {
                if (error) {
                    if (out_err) {
                        NSString *detail = error.userInfo[@"WKJavaScriptExceptionMessage"];
                        if (detail && [detail length] > 0) {
                            *out_err = strdup([detail UTF8String]);
                        } else {
                            *out_err = strdup([error.localizedDescription UTF8String]);
                        }
                    }
                    dispatch_semaphore_signal(sem);
                    return;
                }

                int w = 0, h = 0;
                if ([result isKindOfClass:[NSDictionary class]]) {
                    w = [result[@"width"] intValue];
                    h = [result[@"height"] intValue];
                }
                if (w <= 0 || h <= 0) {
                    if (out_err) {
                        *out_err = strdup("invalid layout dimensions returned by script");
                    }
                    dispatch_semaphore_signal(sem);
                    return;
                }

                if (out_data && [result isKindOfClass:[NSDictionary class]] && result[@"data"]) {
                    NSData *metadata = [NSJSONSerialization dataWithJSONObject:result[@"data"] options:NSJSONWritingFragmentsAllowed error:nil];
                    if (metadata) *out_data = strdup([[[NSString alloc] initWithData:metadata encoding:NSUTF8StringEncoding] autorelease].UTF8String);
                }
                g_sharedEngine.webView.frame = NSMakeRect(0, 0, w, h);
                WKSnapshotConfiguration *snapConfig = [[[WKSnapshotConfiguration alloc] init] autorelease];
                CGFloat backingScale = [NSScreen mainScreen].backingScaleFactor;
                if (backingScale <= 0) backingScale = 1;
                snapConfig.snapshotWidth = @(w * (density > 0 ? density : 2) / backingScale);
                snapConfig.rect = NSMakeRect(0, 0, w, h);
                [g_sharedEngine.webView takeSnapshotWithConfiguration:snapConfig completionHandler:^(NSImage *snapshot, NSError *snapError) {
                    if (snapError || !snapshot) {
                        if (out_err) {
                            *out_err = strdup(snapError ? [snapError.localizedDescription UTF8String] : "failed to capture snapshot");
                        }
                    } else {
                        NSData *tiffData = [snapshot TIFFRepresentation];
                        NSBitmapImageRep *bitmap = [NSBitmapImageRep imageRepWithData:tiffData];
                        NSData *png = [bitmap representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
                        if (png && [png length] > 0) {
                            *out_w = w;
                            *out_h = h;
                            *out_len = [png length];
                            *out_png = malloc(*out_len);
                            memcpy(*out_png, [png bytes], *out_len);
                            success = 1;
                        } else if (out_err) {
                            *out_err = strdup("empty PNG data from snapshot");
                        }
                    }
                    dispatch_semaphore_signal(sem);
                }];
            }];
        }
    });

    dispatch_semaphore_wait(sem, DISPATCH_TIME_FOREVER);
    return success;
    }
}

int glance_nativeweb_session_open(const char *html_cstr, unsigned long long *out_id, char **out_err) {
    @autoreleasepool {
    NSString *html = [NSString stringWithUTF8String:html_cstr];
    __block int success = 0;
    __block unsigned long long sessionID = 0;
    dispatch_semaphore_t sem = dispatch_semaphore_create(0);
    dispatch_async(dispatch_get_main_queue(), ^{
        @autoreleasepool {
            __block OffscreenWebEngine *pendingEngine = nil;
            pendingEngine = [[OffscreenWebEngine alloc] initWithHTML:html completion:^{
                success = (pendingEngine && pendingEngine.isLoaded) ? 1 : 0;
                if (success) {
                    sessionID = (unsigned long long)(uintptr_t)CFBridgingRetain(pendingEngine);
                }
                if (!success && out_err) {
                    *out_err = strdup("native WebKit session failed to load HTML");
                }
                [pendingEngine release];
                dispatch_semaphore_signal(sem);
            }];
        }
    });
    dispatch_semaphore_wait(sem, DISPATCH_TIME_FOREVER);
    if (success && out_id) {
        *out_id = sessionID;
    }
    if (!success) {
        return 0;
    }
    return success;
    }
}

static void glance_nativeweb_snapshot_engine(
    OffscreenWebEngine *engine,
    int width,
    int height,
    int density,
    int *out_w,
    int *out_h,
    void **out_png,
    size_t *out_len,
    char **out_err,
    dispatch_semaphore_t sem,
    int *success
) {
    if (!engine || !engine.isLoaded || width <= 0 || height <= 0) {
        if (out_err) *out_err = strdup("invalid or unloaded native WebKit session");
        dispatch_semaphore_signal(sem);
        return;
    }
    WKSnapshotConfiguration *snapConfig = [[[WKSnapshotConfiguration alloc] init] autorelease];
    snapConfig.rect = NSMakeRect(0, 0, width, height);
    if (density <= 0) density = 2;
    if ([snapConfig respondsToSelector:@selector(setSnapshotWidth:)]) {
        CGFloat backingScale = [NSScreen mainScreen].backingScaleFactor;
        if (backingScale <= 0) backingScale = 1;
        snapConfig.snapshotWidth = @(width * density / backingScale);
    }
    [engine.webView takeSnapshotWithConfiguration:snapConfig completionHandler:^(NSImage *snapshot, NSError *snapError) {
        if (snapError || !snapshot) {
            if (out_err) *out_err = strdup(snapError ? [snapError.localizedDescription UTF8String] : "failed to capture session snapshot");
        } else {
            NSData *tiffData = [snapshot TIFFRepresentation];
            NSBitmapImageRep *bitmap = [NSBitmapImageRep imageRepWithData:tiffData];
            NSData *png = [bitmap representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
            if (png && [png length] > 0) {
                *out_w = width;
                *out_h = height;
                *out_len = [png length];
                *out_png = malloc(*out_len);
                memcpy(*out_png, [png bytes], *out_len);
                *success = 1;
            } else if (out_err) {
                *out_err = strdup("empty PNG data from session snapshot");
            }
        }
        dispatch_semaphore_signal(sem);
    }];
}

int glance_nativeweb_session_eval_snapshot(
    unsigned long long session_id,
    const char *script_cstr,
    int density,
    int *out_w,
    int *out_h,
    void **out_png,
    size_t *out_len,
    char **out_err
) {
    @autoreleasepool {
    NSString *rawScript = [NSString stringWithUTF8String:script_cstr];
    NSString *script = [NSString stringWithFormat:@"return await (%@);", rawScript];
    __block int success = 0;
    dispatch_semaphore_t sem = dispatch_semaphore_create(0);
    dispatch_async(dispatch_get_main_queue(), ^{
        @autoreleasepool {
            OffscreenWebEngine *engine = (__bridge OffscreenWebEngine *)(void *)(uintptr_t)session_id;
            if (!engine || !engine.isLoaded) {
                if (out_err) *out_err = strdup("native WebKit session is closed or not loaded");
                dispatch_semaphore_signal(sem);
                return;
            }
            [engine.webView callAsyncJavaScript:script arguments:@{} inFrame:nil inContentWorld:[WKContentWorld pageWorld] completionHandler:^(id result, NSError *error) {
                if (error) {
                    if (out_err) {
                        NSString *detail = error.userInfo[@"WKJavaScriptExceptionMessage"];
                        *out_err = strdup(detail && detail.length ? [detail UTF8String] : [error.localizedDescription UTF8String]);
                    }
                    dispatch_semaphore_signal(sem);
                    return;
                }
                int width = 0, height = 0;
                if ([result isKindOfClass:[NSDictionary class]]) {
                    width = [result[@"width"] intValue];
                    height = [result[@"height"] intValue];
                }
                if (width <= 0 || height <= 0 || (int64_t)width * (int64_t)height > 4194304) {
                    if (out_err) *out_err = strdup("invalid chart dimensions returned by session script");
                    dispatch_semaphore_signal(sem);
                    return;
                }
                glance_nativeweb_snapshot_engine(engine, width, height, density, out_w, out_h, out_png, out_len, out_err, sem, &success);
            }];
        }
    });
    dispatch_semaphore_wait(sem, DISPATCH_TIME_FOREVER);
    return success;
    }
}

int glance_nativeweb_session_snapshot(
    unsigned long long session_id,
    int width,
    int height,
    int density,
    int *out_w,
    int *out_h,
    void **out_png,
    size_t *out_len,
    char **out_err
) {
    @autoreleasepool {
    __block int success = 0;
    dispatch_semaphore_t sem = dispatch_semaphore_create(0);
    dispatch_async(dispatch_get_main_queue(), ^{
        @autoreleasepool {
            OffscreenWebEngine *engine = (__bridge OffscreenWebEngine *)(void *)(uintptr_t)session_id;
            glance_nativeweb_snapshot_engine(engine, width, height, density, out_w, out_h, out_png, out_len, out_err, sem, &success);
        }
    });
    dispatch_semaphore_wait(sem, DISPATCH_TIME_FOREVER);
    return success;
    }
}

void glance_nativeweb_session_close(unsigned long long session_id) {
    if (session_id == 0) return;
    OffscreenWebEngine *engine = (OffscreenWebEngine *)(uintptr_t)session_id;
    dispatch_semaphore_t sem = dispatch_semaphore_create(0);
    dispatch_async(dispatch_get_main_queue(), ^{
        @autoreleasepool {
            [engine.webView stopLoading];
            engine.webView.navigationDelegate = nil;
            engine.webView = nil;
            dispatch_semaphore_signal(sem);
        }
    });
    dispatch_semaphore_wait(sem, DISPATCH_TIME_FOREVER);
    CFRelease((CFTypeRef)engine);
}

void glance_nativeweb_close(void) {
    dispatch_semaphore_t sem = dispatch_semaphore_create(0);
    dispatch_async(dispatch_get_main_queue(), ^{
        if (g_sharedEngine) {
            WKWebView *wv = g_sharedEngine.webView;
            if (wv) {
                [wv stopLoading];
                wv.navigationDelegate = nil;
                [wv removeFromSuperview];

                g_sharedEngine.webView = nil;
            }
            [g_sharedEngine release];
            g_sharedEngine = nil;
        }
        [g_currentHTML release];
        g_currentHTML = nil;
        dispatch_semaphore_signal(sem);
    });
    // Called by background teardown. Finish clearing this generation before a
    // new default engine can create a replacement page.
    dispatch_semaphore_wait(sem, DISPATCH_TIME_FOREVER);
}

void glance_nativeweb_pump(double seconds) {
    @autoreleasepool {
        [[NSRunLoop currentRunLoop] runMode:NSDefaultRunLoopMode beforeDate:[NSDate dateWithTimeIntervalSinceNow:seconds]];
    }
}
