#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>
#include "_cgo_export.h"

// The settings window: one NSWindow with a WKWebView, in a process that
// exists only while the window is open. Everything here runs on the main
// thread.

static const CGFloat kWidth = 960, kHeight = 640, kMinWidth = 720, kMinHeight = 520;

static NSWindow *gWindow;
static WKWebView *gWebView;
static NSURL *gPageURL;

static BOOL IsSettingsPage(NSURL *url) {
  return url.isFileURL && [url.URLByStandardizingPath.path isEqualToString:gPageURL.URLByStandardizingPath.path];
}

// Serves ags-preview://data/... to the sandboxed preview frame. Go decides
// what may be read (servePreview); this only turns its answer into a
// response.
@interface AGSPreviewScheme : NSObject <WKURLSchemeHandler>
@end

@implementation AGSPreviewScheme
- (void)webView:(WKWebView *)webView startURLSchemeTask:(id<WKURLSchemeTask>)task {
  NSString *range = [task.request valueForHTTPHeaderField:@"Range"] ?: @"";
  PreviewReply reply = {0};
  previewRequest((char *)task.request.URL.absoluteString.UTF8String, (char *)range.UTF8String, &reply);
  NSMutableDictionary *headers = [NSMutableDictionary dictionaryWithDictionary:@{
    @"Accept-Ranges" : @"bytes",
    @"Content-Length" : [NSString stringWithFormat:@"%lld", reply.length],
  }];
  if (reply.mime != NULL) {
    headers[@"Content-Type"] = [NSString stringWithUTF8String:reply.mime];
    free(reply.mime);
  }
  if (reply.status == 206) {
    headers[@"Content-Range"] = [NSString stringWithFormat:@"bytes %lld-%lld/%lld", reply.start, reply.start + reply.length - 1, reply.size];
  } else if (reply.status == 416) {
    headers[@"Content-Range"] = [NSString stringWithFormat:@"bytes */%lld", reply.size];
  }
  [task didReceiveResponse:[[NSHTTPURLResponse alloc] initWithURL:task.request.URL
                                                       statusCode:reply.status
                                                      HTTPVersion:@"HTTP/1.1"
                                                     headerFields:headers]];
  if (reply.body != NULL) {
    [task didReceiveData:[NSData dataWithBytesNoCopy:reply.body length:(NSUInteger)reply.length freeWhenDone:YES]];
  }
  [task didFinish];
}
- (void)webView:(WKWebView *)webView stopURLSchemeTask:(id<WKURLSchemeTask>)task {
  // Every task finishes inside startURLSchemeTask, so there is nothing to stop.
}
@end

@interface AGSSettingsController : NSObject <WKScriptMessageHandler, WKNavigationDelegate, NSWindowDelegate, NSApplicationDelegate>
@end

@implementation AGSSettingsController

// The page posts {id, method, params}; Go gets it as one JSON string.
- (void)userContentController:(WKUserContentController *)controller
      didReceiveScriptMessage:(WKScriptMessage *)message {
  // WebKit gives the "ags" handler to every frame, and the preview frame runs
  // third-party extension scripts. Only the settings page itself may call
  // Go, or an extension could change settings or remove extensions.
  if (!message.frameInfo.isMainFrame || !IsSettingsPage(message.frameInfo.request.URL)) return;
  NSData *json = nil;
  if ([NSJSONSerialization isValidJSONObject:message.body]) {
    json = [NSJSONSerialization dataWithJSONObject:message.body options:0 error:nil];
  }
  if (json == nil) return;
  NSString *text = [[NSString alloc] initWithData:json encoding:NSUTF8StringEncoding];
  settingsPageMessage((char *)text.UTF8String);
}

// The page and its preview are local files; nothing may navigate elsewhere.
- (void)webView:(WKWebView *)webView
    decidePolicyForNavigationAction:(WKNavigationAction *)action
                    decisionHandler:(void (^)(WKNavigationActionPolicy))decisionHandler {
  // file: is only for the page, and the preview scheme only for the preview
  // frame, so the sandboxed frame cannot navigate itself to a file.
  NSString *scheme = action.request.URL.scheme;
  WKFrameInfo *target = action.targetFrame;
  BOOL mainFrame = target != nil && target.isMainFrame;
  BOOL subframe = target != nil && !target.isMainFrame;
  BOOL allowed = (mainFrame && [scheme isEqualToString:@"file"]) ||
                 (subframe && [scheme isEqualToString:@"ags-preview"]) ||
                 [scheme isEqualToString:@"about"];
  decisionHandler(allowed ? WKNavigationActionPolicyAllow : WKNavigationActionPolicyCancel);
}

// The window is shown once the page has drawn, so it never flashes white in
// dark mode. A page that fails to load still shows the window.
- (void)webView:(WKWebView *)webView didFinishNavigation:(WKNavigation *)navigation {
  [self show];
}
- (void)webView:(WKWebView *)webView didFailProvisionalNavigation:(WKNavigation *)navigation withError:(NSError *)error {
  [self show];
}
- (void)show {
  if (gWindow.isVisible) return;
  [gWindow makeKeyAndOrderFront:nil];
  [NSApp activate];
}

// Opening Agent Screensaver again (Spotlight, Launchpad, Finder) while the
// window is open reaches this process as a reopen, because the launcher app
// runs this very process. Bring the one window back instead of doing nothing.
- (BOOL)applicationShouldHandleReopen:(NSApplication *)app hasVisibleWindows:(BOOL)visible {
  if (gWindow.isMiniaturized) [gWindow deminiaturize:nil];
  [gWindow makeKeyAndOrderFront:nil];
  [NSApp activate];
  return NO;
}

- (void)closeWindow:(id)sender {
  [gWindow performClose:sender];
}

// Closing the window ends the process. [NSApp stop:] only takes effect after
// the next event, so post one to wake the loop.
- (void)windowWillClose:(NSNotification *)note {
  [NSApp stop:nil];
  NSEvent *wake = [NSEvent otherEventWithType:NSEventTypeApplicationDefined
                                     location:NSZeroPoint
                                modifierFlags:0
                                    timestamp:0
                                 windowNumber:0
                                      context:nil
                                      subtype:0
                                        data1:0
                                        data2:0];
  [NSApp postEvent:wake atStart:YES];
}
@end

static NSMenuItem *Item(NSString *title, SEL action, NSString *key, NSEventModifierFlags modifiers, id target) {
  NSMenuItem *item = [[NSMenuItem alloc] initWithTitle:title action:action keyEquivalent:key];
  item.keyEquivalentModifierMask = modifiers;
  item.target = target;
  return item;
}

static NSMenuItem *Submenu(NSString *title, NSArray<NSMenuItem *> *items) {
  NSMenu *menu = [[NSMenu alloc] initWithTitle:title];
  for (NSMenuItem *item in items) [menu addItem:item];
  NSMenuItem *holder = [[NSMenuItem alloc] initWithTitle:title action:nil keyEquivalent:@""];
  holder.submenu = menu;
  return holder;
}

// An app without a main menu has no Cmd+C or Cmd+V. The Edit items have no
// target, so they go to the focused text field in the page.
static void InstallMenu(AGSSettingsController *controller, NSString *name) {
  NSEventModifierFlags cmd = NSEventModifierFlagCommand;
  NSMenu *bar = [[NSMenu alloc] init];
  [bar addItem:Submenu(name, @[
    Item([@"Hide " stringByAppendingString:name], @selector(hide:), @"h", cmd, nil),
    [NSMenuItem separatorItem],
    Item([@"Quit " stringByAppendingString:name], @selector(closeWindow:), @"q", cmd, controller),
  ])];
  [bar addItem:Submenu(@"File", @[
    Item(@"Close Window", @selector(closeWindow:), @"w", cmd, controller),
  ])];
  [bar addItem:Submenu(@"Edit", @[
    Item(@"Undo", @selector(undo:), @"z", cmd, nil),
    Item(@"Redo", @selector(redo:), @"z", cmd | NSEventModifierFlagShift, nil),
    [NSMenuItem separatorItem],
    Item(@"Cut", @selector(cut:), @"x", cmd, nil),
    Item(@"Copy", @selector(copy:), @"c", cmd, nil),
    Item(@"Paste", @selector(paste:), @"v", cmd, nil),
    Item(@"Select All", @selector(selectAll:), @"a", cmd, nil),
  ])];
  NSApp.mainMenu = bar;
}

void RunSettingsWindow(const char *title, const char *page, const char *readAccess, const void *icon, long long iconLength) {
  @autoreleasepool {
    [NSApplication sharedApplication];
    // Regular while the window is open: a Dock icon and a menu bar, like
    // any app with a window. The process ends with the window.
    [NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
    // dataWithBytes copies, so the Go bytes are not kept past this call.
    NSImage *appIcon = [[NSImage alloc] initWithData:[NSData dataWithBytes:icon length:(NSUInteger)iconLength]];
    if (appIcon) NSApp.applicationIconImage = appIcon;
    NSString *name = [NSString stringWithUTF8String:title];
    AGSSettingsController *controller = [AGSSettingsController new];
    InstallMenu(controller, name);
    NSApp.delegate = controller;

    WKWebViewConfiguration *config = [WKWebViewConfiguration new];
    [config.userContentController addScriptMessageHandler:controller name:@"ags"];
    [config setURLSchemeHandler:[AGSPreviewScheme new] forURLScheme:@"ags-preview"];
    NSRect frame = NSMakeRect(0, 0, kWidth, kHeight);
    gWebView = [[WKWebView alloc] initWithFrame:frame configuration:config];
    gWebView.navigationDelegate = controller;

    gWindow = [[NSWindow alloc] initWithContentRect:frame
                                          styleMask:(NSWindowStyleMaskTitled | NSWindowStyleMaskClosable |
                                                     NSWindowStyleMaskMiniaturizable | NSWindowStyleMaskResizable)
                                            backing:NSBackingStoreBuffered
                                              defer:NO];
    gWindow.title = name;
    gWindow.contentMinSize = NSMakeSize(kMinWidth, kMinHeight);
    gWindow.releasedWhenClosed = NO;
    gWindow.delegate = controller;
    gWindow.contentView = gWebView;
    [gWindow center];

    gPageURL = [NSURL fileURLWithPath:[NSString stringWithUTF8String:page]];
    NSURL *accessURL = [NSURL fileURLWithPath:[NSString stringWithUTF8String:readAccess] isDirectory:YES];
    [gWebView loadFileURL:gPageURL allowingReadAccessToURL:accessURL];
    [NSApp run];
  }
}

void EvaluateInSettingsWindow(const char *script) {
  [gWebView evaluateJavaScript:[NSString stringWithUTF8String:script] completionHandler:nil];
}
