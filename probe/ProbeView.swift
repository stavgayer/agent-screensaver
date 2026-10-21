import AppKit
import ScreenSaver
import WebKit

// Sandbox probe: which folders can legacyScreenSaver read and load a page from?
// Every result goes on screen (large text, for screenshots) and to NSLog
// with the prefix "AGS-PROBE".

@objc(AgentProbeView)
final class AgentProbeView: ScreenSaverView, WKNavigationDelegate, WKScriptMessageHandler {
    private struct Candidate {
        let id: Int
        let base: String
        var probeDir: String { base + "/probe" }
    }

    private let report = NSTextField(wrappingLabelWithString: "")
    private var lines: [String] = []
    private var webViews: [WKWebView] = []
    private var candidates: [Candidate] = []
    private var started = false

    override init?(frame: NSRect, isPreview: Bool) {
        super.init(frame: frame, isPreview: isPreview)
        animationTimeInterval = 1.0
        report.font = .monospacedSystemFont(ofSize: isPreview ? 9 : 22, weight: .bold)
        report.textColor = .white
        report.backgroundColor = .clear
        report.maximumNumberOfLines = 0
        addSubview(report)
    }

    required init?(coder: NSCoder) { fatalError() }

    override func startAnimation() {
        super.startAnimation()
        guard !started else { return }
        started = true
        runProbe()
    }

    override func draw(_ rect: NSRect) {
        NSColor(calibratedWhite: 0.08, alpha: 1).setFill()
        rect.fill()
    }

    override func layout() {
        super.layout()
        let reportHeight = bounds.height * 0.62
        report.frame = NSRect(x: 16, y: bounds.height - reportHeight, width: bounds.width - 32, height: reportHeight - 8)
        let slot = bounds.width / 3
        for (index, web) in webViews.enumerated() {
            web.frame = NSRect(x: slot * CGFloat(index) + 4, y: 4, width: slot - 8, height: bounds.height - reportHeight - 12)
        }
    }

    // MARK: - Probe

    private func runProbe() {
        let realHome = Self.realHomeDirectory()
        let container = realHome + "/Library/Containers/com.apple.ScreenSaver.Engine.legacyScreenSaver/Data"
        candidates = [
            Candidate(id: 1, base: realHome + "/Library/Application Support/agent-screensaver"),
            Candidate(id: 2, base: "/Users/Shared/agent-screensaver"),
            Candidate(id: 3, base: container + "/Library/Application Support/agent-screensaver"),
        ]

        record("real home (getpwuid): \(realHome)")
        record("NSHomeDirectory(): \(NSHomeDirectory())")

        for candidate in candidates {
            record("C\(candidate.id) read state.json: \(Self.readStatus(candidate.probeDir + "/state.json"))")
        }
        for candidate in candidates {
            record("C\(candidate.id) write: \(Self.writeStatus(candidate.probeDir + "/write-test.txt"))")
        }
        probeAerials()
        for candidate in candidates { loadWebPage(candidate) }
        layout()
    }

    /// Inside the sandbox NSHomeDirectory() is the container, so ask the passwd database.
    private static func realHomeDirectory() -> String {
        guard let entry = getpwuid(getuid()), let dir = entry.pointee.pw_dir else { return "(unknown)" }
        return String(cString: dir)
    }

    private static func readStatus(_ path: String) -> String {
        do {
            _ = try Data(contentsOf: URL(fileURLWithPath: path))
            return "OK"
        } catch {
            return describe(error)
        }
    }

    private static func describe(_ error: Error) -> String {
        let ns = error as NSError
        switch ns.code {
        case NSFileReadNoPermissionError: return "DENIED (\(ns.domain) \(ns.code))"
        case NSFileReadNoSuchFileError, NSFileNoSuchFileError: return "MISSING (\(ns.domain) \(ns.code))"
        default: return "ERROR \(ns.domain) \(ns.code)"
        }
    }

    private static func writeStatus(_ path: String) -> String {
        do {
            try "probe write test\n".write(toFile: path, atomically: false, encoding: .utf8)
            return "OK"
        } catch {
            let ns = error as NSError
            return "DENIED (\(ns.domain) \(ns.code))"
        }
    }

    private func probeAerials() {
        let root = "/Library/Application Support/com.apple.idleassetsd/Customer"
        record("aerial entries.json: \(Self.readStatus(root + "/entries.json"))")

        let videoDir = root + "/4KSDR240FPS"
        do {
            let names = try FileManager.default.contentsOfDirectory(atPath: videoDir)
            guard let mov = names.sorted().first(where: { $0.hasSuffix(".mov") }) else {
                record("aerial .mov open: NONE (no .mov in dir)")
                return
            }
            let handle = try FileHandle(forReadingFrom: URL(fileURLWithPath: videoDir + "/" + mov))
            defer { try? handle.close() }
            let head = try handle.read(upToCount: 16) ?? Data()
            record("aerial .mov open: OK (\(head.count) bytes of \(mov))")
        } catch {
            record("aerial .mov open: \(Self.describe(error))")
        }
    }

    private func loadWebPage(_ candidate: Candidate) {
        let config = WKWebViewConfiguration()
        config.userContentController.add(self, name: "probe")
        let web = WKWebView(frame: .zero, configuration: config)
        web.navigationDelegate = self
        web.tag(candidate.id)
        addSubview(web)
        webViews.append(web)

        let index = URL(fileURLWithPath: candidate.probeDir + "/index.html")
        let readable = Self.readStatus(index.path)
        guard readable == "OK" else {
            record("C\(candidate.id) web page: \(readable) (index.html)")
            return
        }
        web.loadFileURL(index, allowingReadAccessTo: URL(fileURLWithPath: candidate.base))
    }

    // MARK: - WebKit callbacks

    func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
        record("C\(webView.probeID) web page: OK (loaded)")
    }

    func webView(_ webView: WKWebView, didFail navigation: WKNavigation!, withError error: Error) {
        record("C\(webView.probeID) web page: DENIED (\(error.localizedDescription))")
    }

    func webView(_ webView: WKWebView, didFailProvisionalNavigation navigation: WKNavigation!, withError error: Error) {
        record("C\(webView.probeID) web page: DENIED (\(error.localizedDescription))")
    }

    func userContentController(_ controller: WKUserContentController, didReceive message: WKScriptMessage) {
        guard let web = message.webView else { return }
        let word = (message.body as? String) == "sibling-ok" ? "OK" : "MISSING"
        record("C\(web.probeID) sibling script: \(word)")
    }

    // MARK: - Output

    private func record(_ line: String) {
        NSLog("AGS-PROBE %@", line)
        lines.append(line)
        report.stringValue = lines.joined(separator: "\n")
        saveResults()
    }

    /// NSLog is not visible from outside and screencapture fails while the saver runs,
    /// so also keep the results in a file (inside the sandbox: the legacyScreenSaver container).
    /// Rewritten on every result, so async web results land in it too.
    private func saveResults() {
        let header = "\(ISO8601DateFormatter().string(from: Date())) home=\(Self.realHomeDirectory()) isPreview=\(isPreview)"
        let text = ([header] + lines).joined(separator: "\n") + "\n"
        try? text.write(toFile: NSHomeDirectory() + "/agentprobe-results.txt", atomically: true, encoding: .utf8)
    }
}

// WKWebView has no spare field for our candidate id, so keep it in a tag.
private extension WKWebView {
    private static var idKey: UInt8 = 0
    func tag(_ id: Int) { objc_setAssociatedObject(self, &Self.idKey, id, .OBJC_ASSOCIATION_RETAIN) }
    var probeID: Int { (objc_getAssociatedObject(self, &Self.idKey) as? Int) ?? 0 }
}
