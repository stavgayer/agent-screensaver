# Media for the README

The pictures and the video at the top of the main [README](../../README.md). Everything here shows made-up data only: no real prompts, names or paths.

## The files

| File | What it shows | How it was made |
| --- | --- | --- |
| `chrome-running.jpg` | Chrome theme, three agents working, the clock and four limit rings. | Screenshot harness, made-up data. |
| `chrome-approval.jpg` | Chrome theme, Claude Code waiting for approval. | Screenshot harness, made-up data. |
| `chrome-idle.jpg` | Chrome theme with no tasks: the clock and the rings. | Screenshot harness, made-up data. |
| `chrome-secondary.jpg` | Chrome theme on a second display: the background and the clock. | Screenshot harness, made-up data. |
| `demo.gif` | Not recorded yet. A short loop of the demo scene, shown at the top of the README. | The steps below. |

The screenshots come from the screenshot harness in [Test and share](../guides/test-and-share.md), at 1440 by 900, turned into JPEG at quality 80 with `sips -s format jpeg -s formatOptions 80 shot.png --out shot.jpg`. Keep each file well under 400 KB.

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
