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

The saver view runs its page only when it is in a window and one of these is true:
- it is the System Settings thumbnail (`isPreview`), or the host is not legacyScreenSaver (the test harness)
- it is animating, and ScreenSaverEngine is running or this process got `com.apple.screensaver.didstart`

In every other case the web view is hidden, so the page is hidden: the runtime stops its frames and videos, and WebKit suspends the web process. The 1 s state timer stops too. The view checks again on `startAnimation`, `stopAnimation`, `viewDidMoveToWindow`, `didstart`, and when ScreenSaverEngine starts or exits (NSWorkspace launch and terminate notifications).

The spare process itself stays. WallpaperAgent starts it on purpose for the next run. If it quit, WallpaperAgent would only start another one. Paused, it uses no CPU.

## Correction (2026-10-23): the System Settings hosts

The first version of this rule (commit d92f039) also required `isAnimating`, and it required ScreenSaverEngine for every full-size view. After that change, the System Settings preview stayed black. The full run still worked. The diagnostic log from the same owner session shows two causes:
- The thumbnail view (`isPreview`, 111 x 16 window) got `viewDidMoveToWindow` but never `startAnimation`. It got only a `stopAnimation` when Preview was clicked. So it never counted as animating.
- The Preview button does not start ScreenSaverEngine: the log has no ScreenSaverEngine process between 15:11:30 and 15:13:30. Instead, the spare WallpaperAgent process gets `didstart`, makes a new full-size view (`isPreview` false) and animates it, and it gets `willstop` when the Preview ends.

So a preview view now runs whenever it has a window. A full-size view also counts as shown after its process got `didstart`. The process quits at `willstop`, so this state never outlives the run. Spare views stay paused, because they exist before any `didstart` and no engine runs.

## Correction (2026-10-24): the thumbnail draws by itself

After the correction above, the owner saw the full Preview and real runs draw, but the System Settings thumbnail stayed black until Preview was pressed (owner check, 23/10/2026).

Evidence (the 23/10 diagnostic log; we cannot open System Settings ourselves):
- The thumbnail view (`isPreview`) gets `viewDidMoveToWindow` in a 111 x 16 window and starts at 0 x 0. It never gets `startAnimation`. Every view that the host does show (the full Preview, a real run) gets `startAnimation`.
- Our rule already shows the thumbnail's web view and pushes the state, so the page itself runs. What the thumbnail lacks is the host's animation: a `ScreenSaverView` that never animates is the one difference between the black thumbnail and the views that draw.
- Pressing Preview is the first lifecycle call the thumbnail gets (`stopAnimation`), and after it the thumbnail draws.

Change: when a preview view gets a window and is not animating, the view calls `startAnimation()` itself. The view keeps the default `animateOneFrame`, which draws nothing, so this costs only the host's frame timer. The view also keeps the web view on its exact bounds when its size changes, and sends the state again when it first gets a size, so a page that loaded at 0 x 0 is not left without a state.

Spare views are not affected: they are never previews, and they still pause until `didstart` or ScreenSaverEngine.

This is a likely cause, not a proven one. The owner checks that the thumbnail draws on its own when the Screen Saver pane opens. If it is still black, the next step is a diagnostic build that logs the thumbnail's window size, `isVisible` and the page's `visibilityState` over time.

## Alternatives

- Show the page only between `didstart` and `willstop`. This fails for a run whose process starts with the run, because that process misses `didstart` (the first run in the log). Rejected.
- Quit the spare process. WallpaperAgent would start it again, and the next run would start more slowly. Rejected.
- Turn occlusion detection back on. That brings back the black screen (decision 0004). Rejected.

## Risk

If a future macOS shows the saver without ScreenSaverEngine, the full screen saver is black and the preview still works. The owner check after this change covers an idle start and an `open -a ScreenSaverEngine` start.
