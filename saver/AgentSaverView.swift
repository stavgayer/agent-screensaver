import AppKit
import ScreenSaver
import WebKit

// Thin host: one WKWebView that loads the web runtime from DATA_DIR and feeds it
// state.json. The saver only reads DATA_DIR (the sandbox forbids writes, see
// docs/decisions/0001-data-dir.md). The Go daemon is the only writer.

@objc(AgentSaverView)
final class AgentSaverView: ScreenSaverView, WKNavigationDelegate {
    private let webView = WKWebView(frame: .zero, configuration: WKWebViewConfiguration())
    private var timer: Timer?
    private var pageLoaded = false
    private var lastModified: Date?
    private var willStopObserver: NSObjectProtocol?

    override init?(frame: NSRect, isPreview: Bool) {
        super.init(frame: frame, isPreview: isPreview)
        webView.navigationDelegate = self
        webView.setValue(false, forKey: "drawsBackground")
        addSubview(webView)
        observeWillStop()
        loadRuntime()
    }

    required init?(coder: NSCoder) { fatalError("not loaded from a nib") }

    // MARK: - Paths

    /// Inside the sandbox NSHomeDirectory() is the container, so ask the passwd database.
    /// AGS_DATA_DIR is for the test harness only; production uses the real-home path.
    private static let dataDir: URL = {
        if let override = ProcessInfo.processInfo.environment["AGS_DATA_DIR"] {
            return URL(fileURLWithPath: override, isDirectory: true)
        }
        let home = getpwuid(getuid()).flatMap { $0.pointee.pw_dir.map { String(cString: $0) } } ?? NSHomeDirectory()
        return URL(fileURLWithPath: home + "/Library/Application Support/agent-screensaver", isDirectory: true)
    }()

    private static var stateURL: URL { dataDir.appendingPathComponent("state.json") }

    private func loadRuntime() {
        let index = Self.dataDir.appendingPathComponent("runtime/index.html")
        webView.loadFileURL(index, allowingReadAccessTo: Self.dataDir)
    }

    // MARK: - Lifecycle

    override func layout() {
        super.layout()
        webView.frame = bounds
    }

    override func startAnimation() {
        super.startAnimation()
        guard timer == nil else { return }
        timer = Timer.scheduledTimer(withTimeInterval: 1, repeats: true) { [weak self] _ in self?.pushStateIfChanged() }
    }

    override func stopAnimation() {
        super.stopAnimation()
        timer?.invalidate()
        timer = nil
    }

    override func viewDidMoveToWindow() {
        super.viewDidMoveToWindow()
        // The display role may have changed: send the same file again with the new role.
        lastModified = nil
        pushStateIfChanged()
    }

    private func observeWillStop() {
        willStopObserver = DistributedNotificationCenter.default().addObserver(
            forName: Notification.Name("com.apple.screensaver.willstop"), object: nil, queue: .main
        ) { [weak self] _ in self?.shutDown() }
    }

    /// legacyScreenSaver does not always quit after the saver stops (decision 0001), so quit it ourselves.
    /// A preview runs inside System Settings' process tree and must stay alive.
    private func shutDown() {
        timer?.invalidate()
        timer = nil
        webView.stopLoading()
        webView.removeFromSuperview()
        if !isPreview { exit(0) }
    }

    // MARK: - State

    func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
        pageLoaded = true
        lastModified = nil
        pushStateIfChanged()
    }

    private var displayRole: String {
        window?.screen == NSScreen.screens.first ? "main" : "secondary"
    }

    private func pushStateIfChanged() {
        guard pageLoaded else { return }
        let modified = (try? Self.stateURL.resourceValues(forKeys: [.contentModificationDateKey]))?.contentModificationDate
        guard let modified, modified != lastModified else { return }
        guard let json = try? String(contentsOf: Self.stateURL, encoding: .utf8) else { return }
        lastModified = modified
        webView.evaluateJavaScript("window.agentScreensaver.setState(\(Self.jsLiteral(json)), \(Self.jsLiteral(displayRole)))")
    }

    /// A one-element JSON array is valid JS; strip the brackets to get a safe string literal.
    private static func jsLiteral(_ text: String) -> String {
        guard let data = try? JSONSerialization.data(withJSONObject: [text]),
              let array = String(data: data, encoding: .utf8) else { return "\"\"" }
        return String(array.dropFirst().dropLast())
    }
}
