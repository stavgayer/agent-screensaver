# 0001: Data directory for the screen saver

Status: accepted (2026-10-21)

## Context

A macOS screen saver runs inside `legacyScreenSaver`, a sandboxed host. We did not know which folders the saver can read, and whether its web view can load local files from them. We built a probe, ran it inside the real host on macOS 26.6.2 (Apple M4 Pro) on 2026-10-21, and tested three candidate folders.

## Results

| Check | C1 | C2 | C3 |
| --- | --- | --- | --- |
| Read `state.json` | OK | OK | OK |
| Write | DENIED (NSCocoaErrorDomain 513) | OK | OK |
| Sibling script load | OK | OK | OK |
| Web page load | OK | OK | OK |

Candidates:

- C1: `$HOME/Library/Application Support/agent-screensaver` (real home)
- C2: `/Users/Shared/agent-screensaver` (mode 700)
- C3: the `legacyScreenSaver` container

Other checks:

| Check | Result |
| --- | --- |
| `NSHomeDirectory()` inside the sandbox | The container path, not the real home |
| Real home from `getpwuid(getuid()).pw_dir` | `/Users/you` (correct) |
| Aerial `entries.json` read | OK |
| Aerial `.mov` open | NONE (no `.mov` was downloaded at test time) |

## Decision

`DATA_DIR` is candidate 1: `$HOME/Library/Application Support/agent-screensaver`.

Build the path from the real home, `getpwuid(getuid()).pw_dir`. Never use `NSHomeDirectory()`. Inside the sandbox it returns the container, so the saver would look in the wrong place.

Why C1: it is the first candidate where both the file read and the web load passed. It is also the standard macOS place for app data.

## Rules for later tasks

1. The saver must never need to write to `DATA_DIR`. It can read C1 but cannot write there. This fits the design: only the Go daemon (outside the sandbox) writes, and the saver only reads.
2. Always resolve the home folder with `getpwuid(getuid()).pw_dir`, never `NSHomeDirectory()`.

## Fallbacks

If a future macOS version blocks reads from C1, use one of these. Both passed read, write and web load in this run.

- C2: `/Users/Shared/agent-screensaver` (mode 700)
- C3: the `legacyScreenSaver` container

## Open items

- Aerials (closed 2026-10-21): on macOS 26 the aerials live in `~/Library/Application Support/com.apple.wallpaper/aerials/`. The probe, run by the owner inside the real sandbox, read the manifest and a downloaded `.mov`. A `<video>` cannot play the original, because it is outside the folder given to `allowingReadAccessTo`; an APFS clone inside `DATA_DIR` plays (3840 wide). So the daemon clones the chosen videos into `DATA_DIR/media/aerials/`.

## Gotchas

- After you replace a `.saver`, macOS keeps the old code loaded. Quit System Settings (Cmd+Q), then run `killall legacyScreenSaver WallpaperAgent` before you test again.
- `screencapture` cannot capture a running screen saver.
- NSLog lines were not visible in `log show`. The probe writes its results to `<container>/Data/agentprobe-results.txt` instead.
