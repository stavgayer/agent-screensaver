# Media for the README

The pictures and the video at the top of the main [README](../../README.md). Everything here shows made-up data only: no real prompts, names or paths.

## The files

| File | What it shows |
| --- | --- |
| `chrome-working.jpg` | Chrome theme, three agents working. |
| `chrome-approval.jpg` | Chrome theme, Claude Code waiting for approval. |
| `chrome-done.jpg` | Chrome theme, all three tasks finished. |
| `chrome-idle.jpg` | Chrome theme with no tasks: the clock and the rings. |
| `matrix-working.jpg` | Default theme on the Matrix background, three agents working. |
| `matrix-approval.jpg` | Default theme on the Matrix background, waiting for approval. |
| `demo.gif` | Not recorded yet. A short loop of the demo scene, shown at the top of the README. See the steps below. |

There is no picture of a second display: the harness window is always on the main display, and the screen saver takes the secondary role only on a second display. There is no Calm picture because the aerial footage is Apple's.

## Remake the screenshots

The stills come from the real screen saver in the screenshot harness (see [Test and share](../guides/test-and-share.md), section 2), with made-up data from the demo scene (`internal/demo`). The harness window is 1512 by 982 points, so the PNG is 3024 by 1964 on a Retina display.

1. Build: `cd web && npm run build`, `saver/build.sh`, `swiftc -O tools/harness/harness.swift -o build/harness`.
2. Make a temporary data folder `D` with `runtime/` and `extensions/` copied from `build/runtime` and `build/extensions`. Do not use your real data folder.
3. Write `$D/state.json` from the demo scene: a small throwaway Go program inside the repo module (it needs `internal/`) that resolves the view with `view.Resolve`, `view.AgentViews` and `view.AgentOrder` as the service does, takes cards and limits from `demo.Scene(elapsedMs, 60000, nowMs)`, and writes the file with `statefile.Build` and `statefile.WriteAtomic`. Use 3000 ms for working, 12000 ms for approval, 26000 ms for finished, and no cards for idle. The theme is `builtin.chrome` or `builtin.default`. Do not commit the program.
4. Run the harness right after writing the file: `AGS_DATA_DIR="$D" build/harness build/AgentScreensaver.saver 8 shot.png`. It takes the picture halfway through the 8 seconds.
5. Look at each picture. Redo it if it is blank or the fonts are missing.
6. Make the JPEG: `sips --resampleWidth 1440 -s format jpeg -s formatOptions 80 shot.png --out docs/media/name.jpg`. Keep each file well under 400 KB.

## Record the demo video and GIF

You need the installed app and a free copy of [ffmpeg](https://ffmpeg.org) (`brew install ffmpeg`).

1. Pick the theme you want to show, for example `./agent-screensaver theme set builtin.chrome`.
2. Run `./agent-screensaver demo`. It prints "Demo ready. Start the screen saver now". The demo plays for 60 seconds from the moment the screen saver shows. Start it within 5 minutes.
3. Press Cmd+Shift+5, choose "Record Entire Screen" and click Record.
4. Start the screen saver: in the menu bar app, click "Preview screen saver". Then do not touch the mouse or the keyboard, because that stops the screen saver.
5. Wait about 20 seconds. In that time all three cards run, Codex finishes, and Claude Code asks for approval with its sound.

   The macOS screen recording does not capture the Mac's own sound, so the approval sound is not in the video.

6. Press Cmd+Control+Esc to stop the recording. The key press also stops the screen saver. Open the recording in QuickTime Player and use Edit, Trim, to keep only the screen saver part. Save it as `demo.mov`.
7. Make the GIF (12 frames a second, 960 pixels wide):

   ```
   ffmpeg -i demo.mov -t 20 -vf "fps=12,scale=960:-1:flags=lanczos,split[a][b];[a]palettegen[p];[b][p]paletteuse" -loop 0 docs/media/demo.gif
   ```

   Keep it small, a few MB at most. If it is larger, lower `fps` or `scale`.
8. Make the video for GitHub:

   ```
   ffmpeg -i demo.mov -t 20 -vf "scale=1920:-2" -c:v libx264 -pix_fmt yuv420p -movflags +faststart demo.mp4
   ```

9. Commit `docs/media/demo.gif`, and in the README move the GIF line out of its HTML comment.
10. GitHub plays a video in a README only when you upload it in the web editor. On github.com, open `README.md`, click the pencil to edit, and drag `demo.mp4` into the editor at the "Hero video" comment. GitHub uploads it and puts a link in its place. Keep that link on its own line and commit. Do not commit `demo.mp4` or `demo.mov` to the repository.

    GitHub limits the size of a video you upload this way, and the limit may be as low as 10 MB on a free account. Keep the clip short and small. If the file is too large, cut it shorter (lower `-t`) or lower the size (for example `scale=1280:-2`).

`./agent-screensaver demo stop` ends the demo early. A real approval request ends it at once, so record when no agent of yours is busy.
