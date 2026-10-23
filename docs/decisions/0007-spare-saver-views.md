# 0007: Pause saver views that nothing shows

Status: accepted (2026-10-23)

## Context

After a screen saver run stopped, one legacyScreenSaver process stayed alive with AgentScreensaver loaded. It used about 2 to 4% CPU all the time, and its WebKit web and GPU processes used more.

## Evidence

- Unified log: this process is started by WallpaperAgent, not by System Settings. RunningBoard names its host `com.apple.wallpaper.agent`.
- A diagnostic build logged every view's lifecycle in the real host (macOS 26.6.2). The owner ran it for two saver runs, then the System Settings preview, then quit System Settings.
  - When a run stops, our process quits on `willstop`, as before (decision 0001). Less than a second later, WallpaperAgent starts a new legacyScreenSaver. That process makes one or two full-size AgentScreensaver views and calls `startAnimation` on them at once. Nothing shows them.
  - The next run reuses this spare process. It posts `com.apple.screensaver.didstart`, makes the view that it shows, and at `willstop` our process quits. Then WallpaperAgent starts a new spare.
  - From inside the sandbox, the window state does not tell a spare view from a shown one. All views report "not visible" for occlusion. Spare and shown views have the same window class, level and `isVisible` values, and the window server details are not readable.
  - The log shows that ScreenSaverEngine runs exactly while a saver run is on screen. It starts at the run start (`SACScreenSaverStartNow`) and exits at the end.
  - The System Settings preview process quit by itself after Cmd+Q.
- Before decision 0004, WebKit paused these spare pages because their windows reported themselves as covered. Since 0004 ignores that report, the spare pages kept drawing.

## Decision

The saver view runs its page only when all of these are true:
- it is animating
- it is in a window
- it is the System Settings preview, or ScreenSaverEngine is running, or the host is not legacyScreenSaver (the test harness)

In every other case the web view is hidden, so the page is hidden: the runtime stops its frames and videos, and WebKit suspends the web process. The 1 s state timer stops too. The view checks again on `startAnimation`, `stopAnimation`, `viewDidMoveToWindow`, `didstart`, and when ScreenSaverEngine starts or exits (NSWorkspace launch and terminate notifications).

The spare process itself stays. WallpaperAgent starts it on purpose for the next run. If it quit, WallpaperAgent would only start another one. Paused, it uses no CPU.

## Alternatives

- Show the page only between `didstart` and `willstop`. This fails for a run whose process starts with the run, because that process misses `didstart` (the first run in the log). Rejected.
- Quit the spare process. WallpaperAgent would start it again, and the next run would start more slowly. Rejected.
- Turn occlusion detection back on. That brings back the black screen (decision 0004). Rejected.

## Risk

If a future macOS shows the saver without ScreenSaverEngine, the full screen saver is black and the preview still works. The owner check after this change covers an idle start and an `open -a ScreenSaverEngine` start.
