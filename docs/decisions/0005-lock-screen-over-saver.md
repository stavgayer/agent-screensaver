# 0005: Hide the components when the lock screen shows

Status: accepted (2026-10-22)

## Context

When "require password" is on, macOS draws its lock screen over the running saver as soon as the user moves the mouse or presses a key. The lock screen has its own clock and the login field. The saver keeps running behind it, so the screen showed two clocks, and our Codex ring sat next to the login field.

The owner wants this: while the lock screen shows, hide every component and keep the background. If the lock screen goes away and the saver still runs, show the components again.

## Evidence

A diagnostic build of the saver ran in the real host (macOS 26.6.2). It logged, with timestamps, each event that could mean "the lock screen shows":
- distributed notifications
- workspace notifications
- the saver window's occlusion, key and level state
- the CGSession lock flag
- the login windows on screen

In two runs, the owner started the saver, waited, moved the mouse and then unlocked:

| time | event |
|---|---|
| saver starts | `com.apple.screensaver.didstart`, `com.apple.shieldWindowRaised`, `com.apple.screenIsLocked` |
| about 50 s later, mouse moves, lock screen appears | `com.apple.screenLockUIIsShown` |
| unlock | `com.apple.screenLockUIIsHidden`, then 4 ms later `com.apple.screensaver.willstop` |

`screenIsLocked` and `shieldWindowRaised` arrive when the saver starts, not when the lock screen appears, so they cannot be the signal. Only `screenLockUIIsShown` and `screenLockUIIsHidden` change exactly when the lock screen appears and goes away. They reach the sandboxed saver.

## Decision

- The Swift host observes `com.apple.screenLockUIIsShown` and `com.apple.screenLockUIIsHidden`. It calls `window.agentScreensaver.setLockUI(true)` or `setLockUI(false)` through the same `evaluateJavaScript` path as `setState`. It also sends the last value again after a page load.
- The runtime puts the class `lock-ui` on `<html>`. The CSS fades out `#screen` (all component regions) and the service line in 0.25 s. The background is not touched, so it keeps playing. With reduced motion the fade stays, because it has no travel; this is the same rule as the other fades on the page.
- Anything but `true` shows the components, so a bad call can never leave the screen empty.
- If `screenLockUIIsHidden` comes without `willstop` (the lock screen timed out while the saver keeps running), the components come back. The owner's runs did not hit this case: the lock screen stayed until unlock.
- Extensions do not change. Components stay mounted while hidden.

## Alternatives

- `screenIsLocked` / `screenIsUnlocked`: wrong time (sent when the saver starts). Rejected.
- Poll the window occlusion state or the window list: slower, and it depends on the private window layout of loginwindow. Rejected.
- Stop the components while hidden: more work for no visible gain, and they would flash when they mount again. Rejected.

## Risk

These notification names are not documented by Apple. If a future macOS stops sending them, the saver behaves as before (two clocks), and nothing breaks.
