# 0004: The saver ignores window occlusion

Status: accepted (2026-10-22)

## Context

The real `.saver` showed only black in macOS, in the System Settings thumbnail and in the full Preview. The same bundle in the saver harness drew everything.

WebKit decides if a page is visible. One of its checks is the window's occlusion state: if the window says it is covered, WebKit hides the page. A hidden page runs no `requestAnimationFrame`, pauses its videos, and WebKit then suspends the web process and freezes its layers. Nothing draws.

`legacyScreenSaver` draws the saver's window inside another process. Because of this, the window always says it is covered, even when the Preview fills the screen.

## Evidence

- The owner's Preview log (macOS 26.6.2): for each full Preview (1728 x 1117), WebKit logged `window visible 1, window occluded 1` and then `viewIsBecomingInvisible`. It never logged that the view became visible. After that it logged `PrepareToSuspend`, `freezeAllLayerTrees` and `MediaElementSession::suspendBuffering`.
- A diagnostic build in the real host, full Preview, 5 s with the WebKit defaults: `visibilityState` was `hidden`, 0 animation frames, the window occlusion state was "not visible", `setState` ran, no JS errors. Then 5 s with occlusion detection off: `visibilityState` became `visible`, 313 animation frames in 5 s, no errors.

## Decision

`AgentSaverView` turns off WebKit's occlusion check for its web view (`windowOcclusionDetectionEnabled = false`, a private WKWebView setting, set with key-value coding).

The page must still pause when the saver stops. So `stopAnimation` hides the web view, and `startAnimation` shows it again. A hidden view makes the page hidden, so the runtime pauses its frames and videos as before. The runtime's own hidden-page pausing is unchanged.

The web view also follows the size of the saver view (autoresizing). The thumbnail view starts at 0 x 0 and gets its size later.

## Alternatives

- The saver drives frames itself with a native timer that calls into the page. This needs a second frame path in the runtime, and it does not fix the paused videos or the frozen layers. Rejected.
- Make the web view's window report itself as visible. The host owns that window, so the saver cannot change it. Rejected.

## Risk

The setting is private WebKit API. The saver checks that WebKit has the setting before it uses it. If a future macOS removes it, the saver still loads, but it can be black again. Check it on each new major macOS release.
