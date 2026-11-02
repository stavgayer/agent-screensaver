# What extension code can do

A theme is made of extensions: a background, a layout and components. The background and the components run JavaScript. This page says what that code can use, what it cannot do, what it sees, and the rules that every built-in and published theme follows. Read it before you write a theme, and before you install one from someone else.

[extensions.md](extensions.md) describes the manifest and the web API. This page does not repeat them.

## Where your code runs

Your `entry` file runs as a classic script in one web page: the screen saver, or the preview in the settings window. All the extensions of the screen share that page. Each one gets its own element (`root`) to draw in, but the page does not wall them off from each other.

The page loads from the agent-screensaver data folder, `~/Library/Application Support/agent-screensaver`. The screen saver itself runs inside macOS's sandbox for screen savers.

## What your code can use

- The web API: `register`, the view state in `update`, and `ctx` with `requestFrame`, `now`, `options` and `display` ([extensions.md](extensions.md#the-web-api)).
- Files of your own extension folder, by a URL relative to your script: pictures, videos, fonts and more scripts. A script finds its own URL with `document.currentScript.src` while it first runs.
- Other files inside the data folder, by `file:` URL.
- Images as `data:` URLs.
- Two shared helpers, `songProgress` and `recoverWebGL`, on `window.agentScreensaver.helpers` ([extensions.md](extensions.md#shared-helpers)). An extension that uses them declares `"api": 2` and `"requires": "0.5.0"`, so an older app refuses it.
- The DOM inside your `root`, CSS, canvas 2D and WebGL. Ask WebGL for `powerPreference: "low-power"`.

## What your code cannot do

The page has a strict security policy (`default-src 'none'`), so these are blocked:

| Blocked | So |
| --- | --- |
| Any network request: `fetch`, `XMLHttpRequest`, WebSocket, EventSource | Nothing on the screen can send data anywhere or load anything from the internet. Ship every file in your extension folder. |
| Scripts, styles, fonts, images and media from a web address | No CDN, no Google Fonts, no remote pictures. Bundle what you need. |
| Inline `<script>` and `onclick="…"` attributes | Attach handlers from your script instead. |
| `eval`, `new Function` and `setTimeout` with a string | Write plain code. |
| Images from `blob:` URLs, and fonts from `data:` URLs | Draw into a canvas, or ship the file. |
| Frames of any kind | Draw in your own `root`. |

Some limits come from the place your code runs, not from the policy:

- It cannot write files. A web page has no way to, and the screen saver's sandbox can only read the data folder.
- It cannot leave the page. The screen saver refuses any navigation away from its own page (from agent-screensaver 0.4.3), and the preview in the settings window runs in a sandboxed frame.
- It cannot reach the settings. Only the settings page itself can talk to the service. The preview frame, where your code runs, cannot.

## What your code sees

Your code gets the view state: the task cards, the agents, the usage limits, the time and, when the user turns it on, the song that plays ([extensions.md](extensions.md#the-web-api)). That is what the screen shows, so it can include task titles, commands, project names and branches.

When the user turns on privacy mode, the service leaves those out of the state before your code gets it. Your code does not need to hide anything itself, but it must cope with them missing: a card without a title, an approval without a command.

Because nothing on the page can send data out, what your code sees stays on the Mac.

## The rules

Every built-in theme and every theme in [agent-screensaver-themes](https://github.com/stavgayer/agent-screensaver-themes) follows these. Follow them too, so your theme sits well beside the others.

1. **Touch only your own `root`.** Do not change the elements of other extensions, even though the page allows it. Parts of the same theme may read each other's position to share a layout (two album themes place the song between their clock and their limits this way), but never change them, and never read the elements of a built-in or of another theme.
2. **Clean up.** In `unmount` (a component) or `stop` (a background), cancel what `requestFrame` gave you, remove every event listener you added to `window` or `document`, and remove what you drew. The runtime mounts and unmounts your code when the mode or the theme changes.
3. **Draw with `ctx.requestFrame`, at the lowest rate that looks right.** A clock needs 1 frame a second, a moving bar about 4. Do not start your own `requestAnimationFrame` loop or timers that never stop. A screen saver runs for hours.
4. **Show times from the state.** For a time you show, a countdown or a progress bar, use `state.nowMs` and `ctx.now()` between states, not your own reading of the clock, so the screen agrees with the service and the demo. Reading the clock for motion only, such as a slow drift against burn-in, is fine.
5. **Respect "Reduce motion".** With `prefers-reduced-motion: reduce`, stand still or fade, and do not move or scale things.
6. **Handle a lost WebGL context.** After sleep and wake, or a graphics reset, WebGL can lose its context. Listen for `webglcontextlost` and start again on a fresh canvas (the album themes with WebGL do this; `recoverWebGL` from the shared helpers does it for you), or switch to a simple background without WebGL (the built-in Chrome scene does this). Otherwise the background stays black or frozen.
7. **Keep text readable everywhere.** Put small text on a plate, a card or a dark halo, so it reads over every part of your background. Test with long names: cut them with "…" instead of letting them run over other things.
8. **Show only what the state says.** Do not make up numbers or names. A field that is missing is left out.
9. **Use no company logos or artwork you do not own.** The project ships letter marks of its own for the agents. A theme inspired by an album cover draws its look in code, from the cover's shapes and colours, without the artwork.

## Before you install someone else's extension

Installing an extension runs its code on your Mac, with the limits above: it can see what your screen saver shows, but it cannot send it anywhere, load anything from the internet, write files or change your settings. An agent extension is different: it can run programs of its own when your agents send events, so install those only from people you trust. [agents.md](agents.md) describes what they run, and an upgrade that changes those programs shows you the changes before it installs.
