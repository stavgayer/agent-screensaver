# 0003: Tray and settings window libraries

Status: proposed (2026-10-21). Needs the owner's decision, because one of the two libraries named in spec section 7c did not pass the check.

## Context

Spec section 7c names two libraries: `fyne.io/systray` for the tray icon and `github.com/webview/webview_go` for the settings window. Task 5.1 asks us to check that both are maintained and work on macOS 26, before Phase 5 is built on them.

## Maintenance check

Checked on 2026-10-21 with the GitHub API. Today is 2026-10-21, so "last 12 months" means after 2025-10-21.

| Project | Version we use | Last commit | Last release or tag | Open issue about macOS 26 |
| --- | --- | --- | --- | --- |
| `fyne-io/systray` ([repo](https://github.com/fyne-io/systray)) | v1.12.2 (tag date 2026-06-09, from the Go proxy) | 2026-09-13 | v1.12.2, 2026-06-09 | None found. One open macOS bug, [#128](https://github.com/fyne-io/systray/issues/128): `onExit` is not called on macOS. |
| `webview/webview_go` ([repo](https://github.com/webview/webview_go)) | v0.0.0-20240831120633-6173450d4dd6 (no release exists) | 2024-08-31 | none | None found. |
| `webview/webview`, the C/C++ core ([repo](https://github.com/webview/webview)) | not used directly | 2026-10-20 | 0.12.0, 2024-09-11 | None found. Search for "macOS 26" and "Tahoe" gave only old, unrelated macOS issues. |

Facts about `webview_go`:

- Its last commit is about 14 months old. It has no release. The check in the brief ("release or commit in the last 12 months") fails.
- It bundles its own copy of the core header, version 0.11 (`libs/webview/include/webview.h`). The core project has had many commits since, and a 0.12.0 release.
- Open pull requests have waited for a long time: update to webkit2gtk-4.1 ([#62](https://github.com/webview/webview_go/pull/62), January 2025, and [#78](https://github.com/webview/webview_go/pull/78), October 2025), custom user agent on macOS ([#72](https://github.com/webview/webview_go/pull/72), [#77](https://github.com/webview/webview_go/pull/77)). The Linux build is pinned to `webkit2gtk-4.0`, and open issues say it does not build on Ubuntu 24.04 ([#44](https://github.com/webview/webview_go/issues/44), [#50](https://github.com/webview/webview_go/issues/50)). The brief wants Linux support later, so this matters.
- It is not on this machine's Go proxy. It can only be fetched with `GOPROXY=https://proxy.golang.org,direct GOSUMDB=off`. That is a risk for the owner's build setup.

## Spike

Everything was built and run in a scratch folder outside the repo, on an Apple M4 Pro with macOS 26.6.2. Nothing was added to the repo's `go.mod`. Go 1.24.2, `CGO_ENABLED=1`.

### (a) Tray with `fyne.io/systray` v1.12.2

- A program with a template icon (a PNG made in code) and two items: "Settings…" starts the settings program as a child process, and "Quit menu bar app" calls `systray.Quit`.
- Build: arm64 and amd64 built, joined with `lipo`. `lipo -info` shows `x86_64 arm64`. The binary is 9.2 MB. It links only system libraries (Cocoa, AppKit, Foundation, CoreFoundation, libobjc, libresolv, libSystem).
- Run: the menu-ready callback ran (`menu-ready` in the log). A timer stood in for a click on "Settings…": the child started, printed its result, and exited with `nil`. A second timer called `systray.Quit`. `systray.Run` returned and no process was left. We cannot see the screen, so we did not look at the icon itself. The log is the only proof that the menu was created.
- Problem seen: the exit callback passed to `Run` was never called, even on a clean quit. This is [#128](https://github.com/fyne-io/systray/issues/128). Our code must not depend on `onExit`. It can do its cleanup before it calls `systray.Quit`.
- Idle cost, 20 s window with `top -l 3 -s 10 -pid` (the first sample is not a delta, so the last sample is used): 0.0% CPU, 14 MB memory (resident), 9 threads. This was measured after the child had exited.

### (b) Settings window with `webview_go`

- A program with a WKWebView window and two `Bind`-ed Go functions. The page calls `getSettings('builtin.default')` from JavaScript. Go returns a struct, the library turns it into JSON, and the page sends the JSON string back through a second bound function, which prints it to stdout and closes the window after 2 s.
- Result printed: `RESULT {"theme":"builtin.default","sound":true}`, then `window closed`.
- Build: arm64 and amd64 built and joined with `lipo`. The binary is 6.7 MB. The build prints C++ warnings from the library header (`-Wdeprecated-literal-operator`) but no error.
- Run: ran once on this Mac. It also ran as x86_64 under Rosetta (`arch -x86_64`) with the same result.
- So the library works on macOS 26.6.2 today. The problem is not that it fails now. The problem is that nobody maintains it (see above).

### (c) Our own window: cgo with one Objective-C file

Written because (b) does not pass the maintenance check.

- Files: `window_darwin.m` (76 lines), `window_darwin.h` (9), `window_darwin.go` (58), plus a 43-line demo `main.go`. The three window files are 143 lines.
- It makes an `NSWindow` with a `WKWebView`. The page calls `window.webkit.messageHandlers.ags.postMessage(...)` with a JSON string. A `WKScriptMessageHandler` hands the string to an exported Go function. Go decodes it, runs a handler, and answers with `evaluateJavaScript` that calls `window.agsReply(...)`. The page wraps this in a small promise helper so a call looks like `await call('getSettings', ...)`.
- Result printed: `RESULT {"sound":true,"theme":"builtin.default"}`, then `window closed`. The whole run took 3 s.
- Build: arm64 and amd64 built and joined with `lipo`. The binary is 6.2 MB. It compiles with no warning. Ran natively and as x86_64 under Rosetta, same result.
- Two things we had to get right: (1) the Cocoa loop must run on the main thread, so `runtime.LockOSThread()` runs in `init`; (2) `[NSApp stop:]` only takes effect after the next event, so the window delegate posts a dummy event to end the loop.
- Not done in the spike: copy and paste keyboard shortcuts (an app without a main menu has no Cmd+C and Cmd+V), the file picker, window size memory. The core `webview` project has an open issue about shortcuts ([#397](https://github.com/webview/webview/issues/397)), so `webview_go` may have the same gap; we did not test it. A real settings window needs a small Edit menu for these shortcuts.

What the other systems would need behind a `platform` interface (this is an estimate from reading the libraries, not tested):

| System | Window | Bridge in and out |
| --- | --- | --- |
| macOS | `NSWindow` + `WKWebView` (done, about 143 lines) | `WKScriptMessageHandler` in, `evaluateJavaScript` out |
| Windows | A Win32 window with WebView2. Needs the WebView2 runtime (present on Windows 11) and COM calls from C++. Most of the work. | `add_WebMessageReceived` in, `PostWebMessageAsJson` or `ExecuteScript` out |
| Linux | GTK window with WebKitGTK (`webkit2gtk-4.1` or `webkitgtk-6.0`) via cgo and `pkg-config` | `webkit_user_content_manager` script message in, `webkit_web_view_evaluate_javascript` out |

The Go side (`Handler`, JSON requests, replies) would be the same on all three. Each system adds one native file behind one small interface: open window with HTML and size, evaluate script, stop, and a message callback. Expect each non-macOS file to be larger than the macOS one, mostly the Windows one.

## Options

| Option | For | Against |
| --- | --- | --- |
| 1. `webview_go` as the spec says | Works today on macOS 26.6.2. Least code. Same API on three systems. | Last commit 2024-08-31. Bundled core is old (0.11). The Linux build is pinned to webkit2gtk-4.0 and open pull requests to fix it are not merged. Not on the local Go proxy. We would own all fixes anyway. |
| 2. Our own window (c), per system | About 143 lines on macOS. No dependency. Matches the owner's rule of few third-party libraries. We control the bridge and the shortcuts. Easy to test behind an interface. | We write and test the Windows and Linux files ourselves when those systems come. Not tested on a screen by us yet. |
| 3. `webview_go` pinned or vendored as a fork | Less code than option 2 today. | A fork is the same maintenance burden as option 2 with more code we did not write. |
| 4. Use the core `webview/webview` project directly through our own cgo | Core is active (commit 2026-10-19). | It is C++ with its own build rules. It adds a larger native dependency than option 2 needs for our one bridge call. Not tried in the spike. |

## Recommendation

Use `fyne.io/systray` v1.12.2 for the tray. It passed: active, builds universal, runs, quits cleanly, 0.0% CPU idle. Keep one rule in the code: do not rely on the `onExit` callback (issue #128).

Do not use `webview_go`. Build the settings window ourselves (option 2), behind a small `platform` interface, with the macOS file as the first implementation. It is about 143 lines, it already works in the spike, and it removes the one unmaintained dependency and the Linux pin. `webview_go` is the fallback if the owner prefers the least code today and accepts the maintenance risk.

This changes spec section 7c ("the only third-party Go libraries in the project" would become one library, `fyne.io/systray`). The owner must approve the change before Phase 5 starts.
