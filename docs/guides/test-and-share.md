# Test and share an extension

Goal: see your extension work before you share it, then hand it to other people as a folder or a zip.

All commands run as `agent-screensaver` (from the unzipped folder: `./agent-screensaver`). The first three ways to test need no change to your real settings if you use a temporary home folder. Section 2 shows how.

## 1. Dev preview in a browser (fast, for widgets and backgrounds)

The dev page shows the real runtime with sample cards and limits. It switches between idle and attention every 5 seconds. You need a checkout of this repository.

1. Build the web part: `cd web && npm ci && npm run build`.
2. In the repository root, start a static server: `python3 -m http.server 8000`.
3. Open `http://localhost:8000/web/dev/preview.html`.

To put your extension on this page, edit `web/dev/state-idle.json` and `web/dev/state-attention.json`. Both hold a `view` object, the same one the service writes to `state.json`:

- A background: change `view.background`.

```json
"background": { "id": "acme.drift", "entry_url": "{{BUILD}}../docs/guides/examples/acme.drift/main.js", "options": {} }
```

- A widget: add it to `view.components`, and add its `id` to a region of `view.layout.modes.idle` and `attention`.

```json
{ "id": "acme.agent-count", "entry_url": "{{BUILD}}../docs/guides/examples/acme.agent-count/main.js", "options": { "label": "agents working", "size": "large", "show_dot": true } }
```

`{{BUILD}}` is replaced with the address of the `build/` folder. Write the options yourself: the page does not fill in your defaults. Reload the page after each change. Keep these two files as they were when you are done. Do not commit your edits.

The page checks your script, your frame rate, your layout and your look. It does not test the service, the agent hooks or an install.

## 2. The real screen in a test window (the screenshot harness)

The harness runs the real screen saver in a normal window and saves a picture. It reads a data folder you choose, so your real settings stay as they are. You need a checkout, Xcode command line tools and Node.

1. Build once: `cd web && npm ci && npm run build`, then `saver/build.sh`, then `swiftc -O tools/harness/harness.swift -o build/harness`. Also build the service: `go build -o build/agent-screensaver ./cmd/agent-screensaver`.
2. Make a temporary home folder and put the built page and the built-in extensions in its data folder:

```
T=$(mktemp -d); D="$T/Library/Application Support/agent-screensaver"
mkdir -p "$D/extensions" "$D/runtime"
cp -R build/runtime/. "$D/runtime/"; cp -R build/extensions/. "$D/extensions/"
```

3. Install your extension and pick your theme. `HOME=$T` points every command at the temporary folder:

```
HOME=$T build/agent-screensaver ext install docs/guides/examples/acme.dusk
HOME=$T build/agent-screensaver theme set acme.dusk
```

Install what the theme needs first (its background, its layout, its widgets).

4. Send cards. For an agent extension, pipe a recorded payload into its hook. The agent must be installed in the temporary home, but you do not need to add hooks:

```
HOME=$T build/agent-screensaver hook --tool tern --event start < docs/guides/examples/payloads/tern/start.txt
```

5. Start the service, take the picture, stop the service. The harness takes the picture halfway through the seconds you give it:

```
HOME=$T build/agent-screensaver daemon &
AGS_DATA_DIR="$D" build/harness build/AgentScreensaver.saver 8 shot.png
kill %1
```

The service reads waiting events when it starts, so send them first. For the attention layout, send an event of kind `approval`. Cards expire, so send the events right before you start the service.

Do not set `HOME` for your real shell. Put it in front of each command, as above. Do not use the real data folder for tests.

## 3. The settings window

For a real install, the settings window shows the result:

- Theme & background: a live preview of the real runtime, fed with what the service writes now. It shows your theme, your background and your cards. Below it, the theme tiles and the background menu.
- Extension options: a form made from your `options`. Every control saves at once.
- Extensions: "Install..." installs a folder or a `.zip`. "Remove..." removes one (not a built-in one).
- Agents: your agent extension, its state, "Add hooks", and "Add another agent" for an install.

Open it with `agent-screensaver settings`, or from the menu bar app. The preview runs your script in a sandboxed frame, so a script that tries to reach the page around it does not work there. Use only the API in [extensions.md](../extensions.md).

## 4. Install and remove

Install from a folder or a zip. The same command works for both:

```
agent-screensaver ext install ./acme.drift
agent-screensaver ext install ./acme.drift.zip
```

It prints `installed: <id> <version>`. If you install the same `id` again, the new version replaces the old one. The service reads the extensions folder again by itself. To change your code, install it again.

```
agent-screensaver ext list
agent-screensaver ext remove acme.drift
```

`ext list` shows `<id> <type>` for every extension. `ext remove` prints `removed: <id>`. A built-in id cannot be removed. An agent extension has its hooks removed first.

When you remove a theme, `ext remove` also removes its layout, its background and the widgets of that layout. A built-in part stays, and so does a part that another installed theme or layout uses, or the background you chose with `background set`. The command prints `kept: <id> (used by ...)` for it. An agent or another theme is never removed as a part. To remove only the theme, use `ext remove --only <id>`.

If you remove the theme that is in use, the settings go back to `builtin.default`.

## 5. What the install checks refuse

Nothing is left behind when an install is refused. The message says why:

| Problem | Message |
|---|---|
| The `id` starts with `builtin.` | `ids starting with "builtin." are reserved` |
| `api` is not `1` | `unsupported api 2, this build supports 1` |
| The entry file is missing | `entry file: ... main.js: no such file or directory` |
| A symlink in the folder | `"link" is a symlink or special file, which is not allowed` |
| An option default is not valid | `option "count": default: 999 is above the maximum 300` |
| A color is not six hex digits | `must be a color like #rrggbb, got #6cb` |
| A layout has a wrong region or no `attention` mode | `layout: unknown region "left" in mode "idle"`, `layout: missing mode "attention"` |
| The zip has no `extension.json` at the top and more than one top folder | `archive has no extension.json at the top and 2 top folders (one, two): ...` |
| The id does not look like `author.name` | `id "..." must look like "author.name"` |

A zip is also refused when it:

- has more than 1000 entries,
- is more than 50 MiB when unpacked,
- has an entry with an absolute path or a `..` in its path,
- holds a symlink.

For agent extensions there are more checks (protected paths, handler keys, marks, replies, unknown fields). They are in [add-an-agent.md](add-an-agent.md).

A folder is also skipped, without an error, when the service lists extensions and the manifest is not valid, or the folder name is not the same as its `id`. An install always names the folder after the `id`.

## 6. Make a zip

The `extension.json` file must be at the top of the zip, or inside the zip's only top folder. So you can zip the contents of your folder:

```
cd acme.drift
zip -r ../acme.drift.zip .
```

Or you can zip the folder itself, for example with "Compress" in Finder. The install then uses that one folder. The `__MACOSX` folder and the `.DS_Store` files that macOS adds are left out, and they do not count as folders. Keep the zip small: only the files the extension needs. Check it with a temporary home before you share it:

```
HOME=$(mktemp -d) agent-screensaver ext install ./acme.drift.zip
```

## 7. Share a theme as a pack

A theme often needs its own layout, background and widgets. Put each one in its own folder inside one pack folder, with no `extension.json` at the top:

```
acme.dusk-pack/
  theme/
  layout/
  background/
  clock/
```

Zip the pack folder, for example with "Compress" in Finder, or zip its contents:

```
cd acme.dusk-pack
zip -r ../acme.dusk-pack.zip .
```

People install all of it with one command, and remove all of it with one command:

```
agent-screensaver ext install ./acme.dusk-pack.zip
agent-screensaver ext remove acme.dusk
```

The install prints one `installed:` line for each part. If one part is refused, nothing is installed, and the message names the part folder. A pack cannot hold an agent extension. Check the pack with a temporary home before you share it, as in step 6.

## 8. Share it

- Share the zip, or the folder in a repository. Use an `id` with your own name before the dot, for example `yourname.rain`.
- Write in your README what it does, which options it has, and (for an agent) which tool versions you tested.
- Tell people it runs code in the screen saver page, or, for an agent with a converter or a plugin file, that it runs a program of its own. The plan shows a warning for these, and people should read it.
- Do not ship company logos in an agent's mark. Use a letter tile of your own.
- Before you publish, run the install from your zip in a temporary home, and look at one picture of each mode.

The guides in this folder have samples that pass `go test ./internal/settings -run Guide`. Copy a sample and change it.
