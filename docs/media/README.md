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
| `demo.gif` | Chrome theme playing the demo scene for 27.5 seconds: three agents working, Codex finishes, Claude Code asks for approval, then all three finish. 960 by 623, 12 frames a second, shown at the top of the README. |

There is no picture of a second display: the harness window is always on the main display, and the screen saver takes the secondary role only on a second display. There is no Calm picture because the aerial footage is Apple's.

## Remake the screenshots

The stills come from the real screen saver in the screenshot harness (see [Test and share](../guides/test-and-share.md), section 2), with made-up data from the demo scene (`internal/demo`). The harness window is 1512 by 982 points, so the PNG is 3024 by 1964 on a Retina display.

1. Build: `cd web && npm run build`, `saver/build.sh`, `swiftc -O tools/harness/harness.swift -o build/harness`.
2. Make a temporary data folder `D` with `runtime/` and `extensions/` copied from `build/runtime` and `build/extensions`. Do not use your real data folder.
3. Write `$D/state.json` from the demo scene: a small throwaway Go program inside the repo module (it needs `internal/`) that resolves the view with `view.Resolve`, `view.AgentViews` and `view.AgentOrder` as the service does, takes cards and limits from `demo.Scene(elapsedMs, 60000, nowMs)`, and writes the file with `statefile.Build` and `statefile.WriteAtomic`. Use 3000 ms for working, 12000 ms for approval, 26000 ms for finished, and no cards for idle. The theme is `builtin.chrome` or `builtin.default`. Do not commit the program.
4. Run the harness right after writing the file: `AGS_DATA_DIR="$D" build/harness build/AgentScreensaver.saver 8 shot.png`. It takes the picture halfway through the 8 seconds.
5. Look at each picture. Redo it if it is blank or the fonts are missing.
6. Make the JPEG: `sips --resampleWidth 1440 -s format jpeg -s formatOptions 80 shot.png --out docs/media/name.jpg`. Keep each file well under 400 KB.

## Remake the demo GIF and video

The GIF is made the same way as the stills, but the data plays: a small program rewrites the state file from the demo scene as time goes by, and a second harness, `tools/harness/framegrab.swift`, saves a picture about 12 times a second. Nothing shows on your real screen saver, and you need no screen recording. You need [ffmpeg](https://ffmpeg.org) (`brew install ffmpeg`).

1. Build: `cd web && npm run build`, `saver/build.sh`, `swiftc -O tools/harness/framegrab.swift -o build/framegrab`.
2. Make a temporary data folder `D` with `runtime/` and `extensions/` copied from `build/runtime` and `build/extensions`, as for the stills.
3. Write a small throwaway Go program inside the repo module, like the one for the stills, that writes `$D/state.json` again about five times a second. Each time it takes the cards and limits from `demo.Scene(elapsedMs, 60000, nowMs)`, where `elapsedMs` is the time since the program started, and writes the file with `statefile.Build` and `statefile.WriteAtomic`. It prints the start time in milliseconds. The theme is `builtin.chrome`, and the program sets the Chrome scene option `grain` to 0 (a Go float, `0.0`: a plain `0` is an int and the option is ignored; `cfg.Options = map[string]map[string]any{"builtin.chrome-scene": {"grain": 0.0}}`): the moving film grain makes a GIF many times larger. Build it to a folder outside the repo and delete it from the repo. Do not commit it.
4. Start the program for 40 seconds, then at once start the harness for 36 seconds:

   ```
   AGS_DATA_DIR="$D" build/framegrab build/AgentScreensaver.saver 36 12 frames
   ```

   Each picture is named by the time it was taken, in milliseconds. The screen saver reads the state file about once a second, so a card changes up to a second after the scene says so.
5. Make the GIF and the video. Use the start time the program printed. `2.5 30` keeps scene time 2.5 s to 30 s: the fade-in is over at 2.5 s, and at 30 s the scene starts again, so the GIF loops from "all finished" back to "all working".

   ```
   tools/harness/frames-to-gif.sh frames <start ms> 2.5 30 docs/media/demo.gif demo.mp4
   ```

   The GIF is 960 wide with 160 colors, about 7.4 MB. If it grows past 8 MB, lower `max_colors` or the width in the script. The video is H.264, 1280 wide.
6. Look at a few frames of the GIF, for example `ffmpeg -ss 13 -i docs/media/demo.gif -frames:v 1 check.png`: the Chrome scene and its fonts must show, not a black screen.
7. Commit `docs/media/demo.gif`. Do not commit `demo.mp4` or the frames.

## Put the video in the README

GitHub plays a video in a README only when you upload it in the web editor. On github.com, open `README.md`, click the pencil to edit, and drag `demo.mp4` into the editor at the "Hero video" comment. GitHub uploads it and puts a link in its place. Keep that link on its own line and commit. Do not commit `demo.mp4` to the repository.

GitHub limits the size of a video you upload this way, and the limit may be as low as 10 MB on a free account. The video from step 5 is about 1.2 MB.
