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

@interface AGSSettingsController : NSObject <WKScriptMessageHandler, WKNavigationDelegate, NSWindowDelegate>
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
  NSString *scheme = action.request.URL.scheme;
  BOOL local = [scheme isEqualToString:@"file"] || [scheme isEqualToString:@"about"];
  decisionHandler(local ? WKNavigationActionPolicyAllow : WKNavigationActionPolicyCancel);
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

void RunSettingsWindow(const char *title, const char *page, const char *readAccess) {
  @autoreleasepool {
    [NSApplication sharedApplication];
    // Regular while the window is open: a Dock icon and a menu bar, like
    // any app with a window. The process ends with the window.
    [NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
    NSString *name = [NSString stringWithUTF8String:title];
    AGSSettingsController *controller = [AGSSettingsController new];
    InstallMenu(controller, name);

    WKWebViewConfiguration *config = [WKWebViewConfiguration new];
    [config.userContentController addScriptMessageHandler:controller name:@"ags"];
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
