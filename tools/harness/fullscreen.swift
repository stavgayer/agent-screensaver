import AppKit
import ScreenSaver
import WebKit

// Full-screen harness: like harness.swift, but the window covers the whole main
// display (menu bar and Dock included) and hides the cursor, so a screen
// recording shows only the screen saver. It closes itself after <seconds>.
// Usage: fullscreen <saver path> <seconds> <screenshot.png>
// Set AGS_DATA_DIR to point the saver at a temp data dir (test-only override).
// The screenshot is taken at the midpoint of the run.

guard CommandLine.arguments.count >= 4 else {
    print("usage: harness <saver> <seconds> <screenshot.png>")
    exit(2)
}
let path = CommandLine.arguments[1]
let seconds = Double(CommandLine.arguments[2]) ?? 10
let screenshotPath = CommandLine.arguments[3]

let app = NSApplication.shared
app.setActivationPolicy(.regular)
guard let bundle = Bundle(path: path), bundle.load(),
      let viewClass = bundle.principalClass as? ScreenSaverView.Type else {
    print("load fail")
    exit(1)
}
let frame = NSScreen.main!.frame
let view = viewClass.init(frame: frame, isPreview: false)!
let window = NSWindow(contentRect: frame, styleMask: [.borderless], backing: .buffered, defer: false)
window.contentView = view
window.level = .screenSaver
window.setFrame(frame, display: true)
NSCursor.hide()
window.makeKeyAndOrderFront(nil)
app.activate(ignoringOtherApps: true)
view.startAnimation()
print("started pid", getpid())
fflush(stdout)

func findWebView(in root: NSView) -> WKWebView? {
    if let web = root as? WKWebView { return web }
    for child in root.subviews { if let found = findWebView(in: child) { return found } }
    return nil
}

func savePNG(_ image: NSImage) {
    guard let tiff = image.tiffRepresentation, let rep = NSBitmapImageRep(data: tiff),
          let png = rep.representation(using: .png, properties: [:]) else { print("png encode failed"); return }
    do { try png.write(to: URL(fileURLWithPath: screenshotPath)); print("screenshot", screenshotPath) } catch { print("write failed:", error) }
}

DispatchQueue.main.asyncAfter(deadline: .now() + seconds / 2) {
    guard let web = findWebView(in: view) else { print("no web view"); return }
    web.takeSnapshot(with: nil) { image, error in
        if let image { savePNG(image) } else { print("snapshot failed:", error as Any) }
    }
}
DispatchQueue.main.asyncAfter(deadline: .now() + seconds) { view.stopAnimation(); exit(0) }
app.run()
