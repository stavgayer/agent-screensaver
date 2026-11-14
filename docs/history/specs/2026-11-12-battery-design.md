# Battery: design

Date: 2026-11-12. Status: draft, for owner approval.

## Why

People run long agent tasks on a laptop and walk away. If the laptop is not plugged in, the battery can run out. Then the Mac sleeps and every agent stops in the middle of its task. The screen saver exists to tell you from across the room when you must come back. A battery that is about to run out while agents still work is such a moment. Today the screen does not show it.

## Goals

- Show the battery on the idle screen when the Mac runs on battery.
- Warn when the battery is low and at least one agent still works: on the screen, with the menu bar attention icon, and with the attention sound.
- Show nothing about the battery when the Mac is plugged in or has no battery (a desktop Mac).

## Non-goals

- No setting for the threshold. It is fixed at 20%.
- No change to the frame rate on battery. This can be a later feature.
- No battery on the second display.
- The album themes in agent-screensaver-themes do not draw the battery in this change. They can follow later, as with the music widget.

## What the user sees

- **On battery, battery not low:** one more ring next to the usage rings, named "Battery". The center shows the percent that is left, for example `64%`. The detail shows the time that is left, for example `2h 10m left`, when macOS gives it.
- **On battery, low, an agent works:** the battery ring has the `high` color. Under the rings, one line says `Battery 18%: plug in, 2 agents are working`. The menu bar icon shows the attention dot. The attention sound plays one time, when the warning starts, if the screen saver shows and sound is on.
- **On battery, low, no agent works:** the ring has the `high` color. There is no line, no attention dot and no sound. Nothing will be lost if the Mac sleeps.
- **Plugged in, or no battery:** no ring, no line.

## Rules

- **Low:** on battery and 20% or less left.
- **An agent works:** a card is not finished, or a finished card still has background jobs. A card that waits for approval counts as working, because the agent continues after you approve.
- **Warn:** low and at least one agent works.

The daemon computes these rules one time and writes the result to `state.json`. The tray and the web code only read the result. Today the attention rule for approvals is computed in four places. This change does not add a fifth copy.

## How it works

### Read the battery

- A new reader, `platform.Battery`, reads the power source from IOKit (`IOPSCopyPowerSourcesInfo`) through cgo, in `internal/platform/battery_darwin.go`. It needs no permission and starts no process.
- It returns: has a battery, on battery, percent left, minutes left (optional; macOS gives -1 while it calculates), charging.
- The daemon calls it from `Step` every 10 seconds. The call is fast (less than 1 ms), so it runs inline, not in a goroutine.
- The daemon gets the reader through an interface, as it gets `Music`. Tests use a fake reader.

### state.json

A new optional top-level field `power`. The daemon writes it only when the Mac has a battery and runs on battery:

```json
"power": {
  "percent": 18,
  "minutes_left": 34,
  "charging": false,
  "low": true,
  "warn": true,
  "working": 2
}
```

- `minutes_left` is left out when macOS does not know it.
- `working` is the number of cards that work. The warning line uses it.
- The daemon writes `state.json` again when `percent`, `low` or `warn` changes, or when the Mac goes on or off battery. Other changes wait for the periodic rewrite (30 seconds).
- Privacy mode shows the battery. The battery says nothing about your work.

### Attention and sound

- The daemon attention becomes: an approval waits, **or** `power.warn`.
- The sound uses the same rising-edge rule (`ShouldPlaySound`). If an approval already caused the attention, the battery warning does not play the sound again.
- The tray shows the attention icon when an approval waits or `power.warn` is true. The menu gets one line: `Battery low: plug in`.
- The screen mode (`mode.ts`) does not change. The large approval card stays only for approvals. The battery warning is the ring color and the line under the rings.

### Themes

- `web/src/plan.ts` maps `power` to `state.power` in the view state.
- The shared rings component (`web/src/builtins/shared/rings.ts`) adds the battery ring after the usage rings when `state.power` exists. So Default, Calm and Chrome all get it. The ring percent is the battery left, so its color levels are the opposite of the usage rings: `warn` at 30% or less, `high` at 20% or less.
- The rings component draws the warning line when `state.power.warn` is true.
- `docs/extensions.md` documents the `Power` type and `state.power` next to `nowPlaying`. The field is optional, so themes that do not know it keep working. No `api` or `requires` change.

### Demo

The demo (`agent-screensaver demo`) shows the battery on battery at 64%, `1h 50m left`, not low. The demo does not show the warning, so it cannot play the sound.

## Tests

- Daemon: the low, working and warn rules, with a fake reader (on battery, plugged in, no battery, low with and without working cards, finished card with background jobs).
- Daemon: the sound plays one time on the rising edge of the warning, and not again if an approval already caused attention.
- Daemon: `state.json` has no `power` when plugged in or with no battery.
- Tray: the attention icon and the menu line for `power.warn`.
- Web: the battery ring view (percent, time left, levels) and the warning line.
- One manual check on a real MacBook: unplug, see the ring; plug in, the ring goes away within 10 seconds.

## Release

A new minor version, v0.6.0, because `state.json` gets a new field and themes get a new ring. The CHANGELOG gets a section for it.

## Calls I made (owner can change them)

- The threshold is 20%, and there is no setting.
- No toggle in Settings. The ring shows only on battery, so it does not take space on a plugged-in Mac.
- An approval card counts as working.
- The battery shows in privacy mode.
- IOKit through cgo, not `pmset -g batt`. It needs no process and no text parsing.

## Owner change (2026-11-14): show the battery when plugged in

The owner tested the build on battery and asked to keep the ring when the laptop is plugged in, and to say "Charging" while it charges. This replaces "Plugged in, or no battery: no ring, no line" above.

- **Plugged in, charging:** the ring stays. The center shows the percent. The detail says `Charging`. The ring has the charging color: blue in Default and Calm, the theme accent in Chrome (Chrome has no blue in its palette).
- **Plugged in, not charging** (full, or macOS holds the charge to protect the battery): the ring stays with the charging color. The detail says `Full` at 100%, else `Plugged in`.
- **No battery (a desktop Mac):** no ring, as before.
- **Low and warn need battery power.** A plugged-in Mac at 15% is not low and never warns, because nothing will be lost.
- `state.json` `power` gets one more field, `on_battery` (bool). `power` is now written whenever the Mac has a battery.
- The daemon writes the file again when `on_battery` or `charging` changes, so plugging in and out shows within 10 seconds.

Calls I made: blue `#4aa8ff` for Default and Calm; Chrome uses its `--accent`; `Full` and `Plugged in` as the two non-charging captions.
