# Settings window design notes

The settings window (`agent-screensaver settings`, opened from the menu bar app) is a screen people work in, not a page they read. It must feel like the other settings windows on the Mac. This file records the design decisions.

Sources for the criteria: the `emil-design-eng` skill (motion and polish), the `impeccable` skill (Operate mode, craft floor) and the `design-taste-frontend` skill (its app-screen rules, section "Forms and settings").

## The question the window answers

"What will my screen saver look like, and how do I change it?" So the first view shows the real screen saver first, then the choices that change it.

## Structure

- A sidebar with five views: Theme & background, Extension options, Extensions, Connections, General. One view per area, with the area's name as the title. One long page of every area would put General four screens down.
- The title and the save status stay at the top while the view scrolls, so an error is never out of sight.
- Each view is one centred column, at most 760px wide, so a wide window keeps equal margins.

## Theme & background

- One live preview: the real runtime page in a frame (`runtime/preview.html`), fed with `state.json`. It shows what the daemon draws now, with the user's real cards. It is not a mock.
- Only one preview runs. A preview per theme card would run one runtime per theme, each with its own Matrix rain. The other themes are static tiles: a black screen with a clock underlined in the theme's own `--accent`. The tile is drawn from the theme's real style values, not from a stock picture.
- The preview exists only while this view is open and the window is visible. It is removed when the view changes or the window is hidden (`visibilitychange`), so a settings window in the background costs no frames. While it exists, the page reads the state once a second and sends it only when it changed.
- The theme gallery is a group of real radio buttons, so arrow keys move between themes. The background is a popup menu, because a list of five names does not need pictures.
- The frame is sandboxed (`sandbox="allow-scripts"`, an opaque origin) and served from `ags-preview://data/...`, which the window maps to files inside the data folder only. Extension scripts run in that frame, so they must not reach the page or its bridge to Go. In WebKit a sandboxed `file://` frame can load no `file://` script, so the preview cannot use `file://`. The page posts the state into the frame, with data-folder file URLs rewritten to the scheme, and a small script inside the frame passes it to the runtime.

## Forms from option schemas

- `form.ts` `formFor` is the only place that maps an option kind to a control: string to a text field, number to a number field with its range, boolean to a switch, color to a color well, enum to a popup menu, file to "Choose…" with the current file name. A new extension gets its settings with no new page code.
- A number field shows its range under the label ("1 to 240"), so the user knows the limits before an error.
- Every control saves itself when it changes. There is no Save button. One save model per view.

## Look

- The system face (SF Pro) at 13px, the macOS size for settings. Sizes: 12 (descriptions), 13 (labels), 17 (preview title), 20 (view title). Weights 400, 500, 600.
- Grouped panels: a white (dark: #2c2c2f) panel with a 1px line on a grey ground, rows split by a 1px line, label on the left, control on the right. A border, never a border and a shadow.
- Light and dark follow the system (`prefers-color-scheme`). The accent is the system accent (`AccentColor`), so the window follows the user's choice in System Settings.
- The active sidebar item is a neutral grey fill, not a colored pill.
- Secondary text is #636366 in light (6.0:1 on the panel, 5.4:1 on the ground) and #a6a6ab in dark (5.7:1 on the panel). Every tier with words passes 4.5:1.
- The switch is a checkbox drawn as the macOS switch. It keeps checkbox behavior for the keyboard and for VoiceOver (`role="switch"`).

## Connections

- One row per agent extension, built-ins first: the mark in a fixed 24px slot (so names share one edge), the name, "Built in" or "From <id> <version>" with the last event under it, then one status pill and two buttons in fixed columns, so the states and buttons line up down the list.
- One status mark per row. Only "Active" has color (a pale green pill with a dot); every other state is a neutral grey pill. "Hooks were changed outside agent-screensaver" is a second line in the notice color, because it is a separate fact that asks for an action.
- The state text comes from `connections.ts`, which reuses the cards' `formatDuration`, so "4m" means the same on the screen saver and here. The CLI's `agent list` writes the same text.
- Adding or removing hooks changes the user's own files, so it opens a modal sheet (`<dialog>`) with every plan line, Cancel focused. This is the one place in the window that interrupts: it protects a change outside our data folder. Warning lines (an outside agent's own program or plugin file) are tinted, a sentence above them says the agent runs code of its own, and the confirm button says "I trust <name>, add hooks".
- When a file changed between showing and applying, Go refuses by plan hash and the sheet opens again with the new plan and one sentence saying why.
- While the view is open and the window is visible, it reads the states again every 5 seconds. It redraws only when the text would change, and never under an open sheet.
- The sheet fades in from 97% scale in 180 ms (strong ease-out); reduced motion turns it off. It is seen rarely, so it may move; the rows do not.

## Feedback and errors

- After a change, "Saved" shows in the header for 2 seconds and fades (opacity, 150 ms). The status line has a fixed height, so it never moves the page.
- When the service refuses a change, the header says "Not saved" and the service's message shows in red under the control that made the change. The control shows the stored value again.
- Removing an extension takes two clicks: "Remove…" turns into "Cancel" and "Remove <name>". Built-in extensions show "Built in" and have no Remove button.
- The page is rebuilt from a fresh snapshot after every change. Focus and scroll are carried over by element id, so a keyboard user stays on the control they changed.

## Motion

- Press feedback on buttons (scale 0.97, 120 ms, strong ease-out), the switch knob (150 ms), the status fade. Nothing else moves: these are controls people use many times, and motion would make them feel slow.
- `prefers-reduced-motion` turns these transitions off.

## Not done, on purpose

- No phone layout. The window has a minimum size of 720 by 520 points, so the design checker's 390px case cannot happen.
- No picture thumbnails of themes. Themes do not ship a picture, and a made-up one would not match what the theme draws. The live preview shows the real thing.
