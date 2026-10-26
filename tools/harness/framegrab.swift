import AppKit
import ScreenSaver
import WebKit

// Frame-capture harness: runs a .saver in a normal window outside the real
// host, like harness.swift, and saves a PNG frame about every 1/fps seconds.
// Usage: framegrab <saver path> <seconds> <fps> <out dir>
// Set AGS_DATA_DIR to point the saver at a temp data dir (test-only override).
// Each frame is named by the wall-clock milliseconds when it was asked for
// (<out dir>/<ms>.png), so a later step can map frames to scene time and
// fill any frame that came late.

guard CommandLine.arguments.count >= 5 else {
    print("usage: framegrab <saver> <seconds> <fps> <out dir>")
    exit(2)
}
let path = CommandLine.arguments[1]
let seconds = Double(CommandLine.arguments[2]) ?? 30
let fps = Double(CommandLine.arguments[3]) ?? 12
let outDir = CommandLine.arguments[4]
try? FileManager.default.createDirectory(atPath: outDir, withIntermediateDirectories: true)

let app = NSApplication.shared
app.setActivationPolicy(.regular)
guard let bundle = Bundle(path: path), bundle.load(),
      let viewClass = bundle.principalClass as? ScreenSaverView.Type else {
    print("load fail")
    exit(1)
}
let frame = NSRect(x: 0, y: 0, width: 1512, height: 982)
let view = viewClass.init(frame: frame, isPreview: false)!
let window = NSWindow(contentRect: frame, styleMask: [.borderless], backing: .buffered, defer: false)
window.contentView = view
window.level = .floating
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

let encodeQueue = DispatchQueue(label: "encode", qos: .userInitiated, attributes: .concurrent)
let encodeGroup = DispatchGroup()
var pending = false
var saved = 0
var skipped = 0

func save(_ image: NSImage, ms: Int64) {
    guard let cg = image.cgImage(forProposedRect: nil, context: nil, hints: nil) else { return }
    encodeGroup.enter()
    encodeQueue.async {
        defer { encodeGroup.leave() }
        let rep = NSBitmapImageRep(cgImage: cg)
        guard let png = rep.representation(using: .png, properties: [:]) else { return }
        try? png.write(to: URL(fileURLWithPath: "\(outDir)/\(ms).png"))
    }
}

func grab() {
    // Skip a tick while the last snapshot is still on its way, so frames
    // never pile up; the gap is filled when the frames are assembled.
    guard !pending, let web = findWebView(in: view) else { skipped += 1; return }
    pending = true
    let ms = Int64(Date().timeIntervalSince1970 * 1000)
    web.takeSnapshot(with: nil) { image, _ in
        pending = false
        if let image { save(image, ms: ms); saved += 1 }
    }
}

let timer = Timer(timeInterval: 1 / fps, repeats: true) { _ in grab() }
RunLoop.main.add(timer, forMode: .common)
DispatchQueue.main.asyncAfter(deadline: .now() + seconds) {
    timer.invalidate()
    view.stopAnimation()
    encodeGroup.notify(queue: .main) {
        print("frames saved", saved, "skipped ticks", skipped)
        exit(0)
    }
}
app.run()
