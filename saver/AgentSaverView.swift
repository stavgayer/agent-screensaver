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
    private var hostObservers: [NSObjectProtocol] = []
    private var workspaceObservers: [NSObjectProtocol] = []
    private var lockUIShown = false

    override init?(frame: NSRect, isPreview: Bool) {
        super.init(frame: frame, isPreview: isPreview)
        webView.navigationDelegate = self
        webView.setValue(false, forKey: "drawsBackground")
        // legacyScreenSaver's window always reports itself as covered, so WebKit would hide
        // the page and pause it: black screen. Ignore the window; we hide the view ourselves
        // in stopAnimation (decision 0004).
        if Self.canIgnoreOcclusion {
            webView.setValue(false, forKey: "windowOcclusionDetectionEnabled")
        }
        // The thumbnail view starts at 0 x 0 and grows later; the web view follows it.
        webView.frame = bounds
        webView.autoresizingMask = [.width, .height]
        addSubview(webView)
        observeHost()
        observeEngine()
        loadRuntime()
    }

    required init?(coder: NSCoder) { fatalError("not loaded from a nib") }

    deinit { removeHostObservers() }

    /// Checked once per process; a missing setter means the page may be black again (decision 0004).
    private static let canIgnoreOcclusion: Bool = {
        let available = WKWebView.instancesRespond(to: NSSelectorFromString("_setWindowOcclusionDetectionEnabled:"))
        if !available { NSLog("AgentScreensaver: WKWebView has no _setWindowOcclusionDetectionEnabled:, the page may stay hidden") }
        return available
    }()

    // MARK: - Paths

    /// Inside the sandbox NSHomeDirectory() is the container, so ask the passwd database.
    /// There is deliberately no fallback to NSHomeDirectory(): it would load nothing, silently.
    /// AGS_DATA_DIR is for the test harness only; production uses the real-home path.
    private static let dataDir: URL? = {
        if let override = ProcessInfo.processInfo.environment["AGS_DATA_DIR"] {
            return URL(fileURLWithPath: override, isDirectory: true)
        }
        guard let entry = getpwuid(getuid()), let home = entry.pointee.pw_dir else {
            NSLog("AgentScreensaver: cannot resolve home")
            return nil
        }
        return URL(fileURLWithPath: String(cString: home) + "/Library/Application Support/agent-screensaver", isDirectory: true)
    }()

    private static var stateURL: URL? { dataDir?.appendingPathComponent("state.json") }

    private func loadRuntime() {
        guard let dataDir = Self.dataDir else { return }
        webView.loadFileURL(dataDir.appendingPathComponent("runtime/index.html"), allowingReadAccessTo: dataDir)
    }

    // MARK: - Lifecycle

    override func startAnimation() {
        super.startAnimation()
        // The lock screen cannot be up when the saver starts; a missed ...Hidden (the System
        // Settings preview never quits) must not leave the components hidden.
        setLockUI(false)
        updateRunning()
    }

    override func stopAnimation() {
        super.stopAnimation()
        updateRunning()
    }

    override func viewDidMoveToWindow() {
        super.viewDidMoveToWindow()
        // The display role may have changed: send the same file again with the new role.
        lastModified = nil
        pushStateIfChanged()
        updateRunning()
    }

    private static let engineBundleID = "com.apple.ScreenSaver.Engine"

    /// WallpaperAgent starts a spare legacyScreenSaver after each run and animates its views
    /// although nothing shows them (decision 0007). A full-screen view is on screen while
    /// ScreenSaverEngine runs, or after didstart (the System Settings Preview button runs the
    /// saver in the spare process without the engine). The process quits at willstop.
    private static var engineRunning: Bool {
        !NSRunningApplication.runningApplications(withBundleIdentifier: engineBundleID).isEmpty
    }

    /// Only legacyScreenSaver makes spare views; any other host (the test harness) shows its view.
    private static let hostMakesSpares = Bundle.main.bundleIdentifier == "com.apple.ScreenSaver.Engine.legacyScreenSaver"

    private static var runStarted = false

    /// The System Settings thumbnail never gets startAnimation, so a preview runs while it has a window.
    private var shouldRun: Bool {
        guard window != nil else { return false }
        if isPreview || !Self.hostMakesSpares { return true }
        return isAnimating && (Self.runStarted || Self.engineRunning)
    }

    /// Pauses the page when nothing shows it: a hidden view makes the page hidden, so the
    /// runtime stops its frames and videos and WebKit can suspend the web process.
    private func updateRunning() {
        let run = shouldRun
        webView.isHidden = !run
        if run, timer == nil {
            timer = Timer.scheduledTimer(withTimeInterval: 1, repeats: true) { [weak self] _ in self?.pushStateIfChanged() }
            lastModified = nil
            pushStateIfChanged()
        } else if !run {
            timer?.invalidate()
            timer = nil
        }
    }

    private func observeEngine() {
        let center = NSWorkspace.shared.notificationCenter
        let names = [NSWorkspace.didLaunchApplicationNotification, NSWorkspace.didTerminateApplicationNotification]
        workspaceObservers = names.map { name in
            center.addObserver(forName: name, object: nil, queue: .main) { [weak self] note in
                let app = note.userInfo?[NSWorkspace.applicationUserInfoKey] as? NSRunningApplication
                if app?.bundleIdentifier == Self.engineBundleID { self?.updateRunning() }
            }
        }
    }

    /// loginwindow posts screenLockUIIsShown when its lock screen (clock, login) appears over
    /// the running saver, and screenLockUIIsHidden when it goes away (decision 0005).
    private func observeHost() {
        let center = DistributedNotificationCenter.default()
        let handlers: [(String, (AgentSaverView) -> Void)] = [
            ("com.apple.screensaver.willstop", { $0.shutDown() }),
            ("com.apple.screensaver.didstart", { Self.runStarted = true; $0.updateRunning() }),
            ("com.apple.screenLockUIIsShown", { $0.setLockUI(true) }),
            ("com.apple.screenLockUIIsHidden", { $0.setLockUI(false) }),
        ]
        hostObservers = handlers.map { name, handle in
            center.addObserver(forName: Notification.Name(name), object: nil, queue: .main) { [weak self] _ in
                if let self { handle(self) }
            }
        }
    }

    private func removeHostObservers() {
        let center = DistributedNotificationCenter.default()
        hostObservers.forEach(center.removeObserver)
        hostObservers = []
        let workspace = NSWorkspace.shared.notificationCenter
        workspaceObservers.forEach(workspace.removeObserver)
        workspaceObservers = []
    }

    private func setLockUI(_ shown: Bool) {
        lockUIShown = shown
        pushLockUI()
    }

    private func pushLockUI() {
        guard pageLoaded else { return }
        webView.evaluateJavaScript("window.agentScreensaver.setLockUI(\(lockUIShown))")
    }

    /// legacyScreenSaver does not always quit after the saver stops (decision 0001), so quit it ourselves.
    /// A preview runs inside System Settings' process tree and must stay alive.
    private func shutDown() {
        removeHostObservers()
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
        pushLockUI()
    }

    private var displayRole: String {
        window?.screen == NSScreen.screens.first ? "main" : "secondary"
    }

    private func pushStateIfChanged() {
        guard pageLoaded else { return }
        guard let stateURL = Self.stateURL else { return }
        let modified = (try? stateURL.resourceValues(forKeys: [.contentModificationDateKey]))?.contentModificationDate
        guard let modified, modified != lastModified else { return }
        guard let json = try? String(contentsOf: stateURL, encoding: .utf8) else { return }
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
