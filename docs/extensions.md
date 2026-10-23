# Writing extensions

This page tells you how to write an extension for agent-screensaver and how to install it.

## The six types

| Type | What it does | Has a manifest |
|---|---|---|
| Background | Draws the full-screen background. | yes |
| Component | Draws one piece of the screen, such as a clock or a card row. | yes |
| Layout | Says where components go on the screen. It is data only. | yes |
| Theme | Picks a background, a layout and style values. | yes |
| Agent | Teaches the service one coding agent: its mark, how its hook payloads become events, how to add its hooks, where to read card details. See [agents.md](agents.md). | yes |
| Source | Sends cards and limits into the service. | no |

A source is not an extension folder. It is any program that sends events. See [event-format.md](event-format.md).

## The folder

An extension is a folder. The folder holds a file named `extension.json` and any files it needs. Install it from a folder or from a `.zip` file. The `extension.json` file must be at the top of the folder or the zip, not inside a sub-folder.

```
acme.rain/
  extension.json
  main.js
```

## The manifest

`extension.json` is one JSON object.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `id` | string | yes | Unique name, like `acme.rain`. See the rules below. |
| `name` | string | no | Name shown to people. |
| `version` | string | no | Your version, for example `1.0.0`. |
| `type` | string | yes | `background`, `component`, `layout`, `theme` or `agent`. |
| `api` | number | yes | The API version you need. It must be `1`. |
| `entry` | string | background, component | The JavaScript file to load. |
| `options` | object | no | Settings people can change. See below. |
| `layout` | object | layout | The layout. See below. |
| `theme` | object | theme | The theme. See below. |

Rules:

- The `id` uses lowercase letters, digits and dashes. It has at least one dot. Examples: `acme.rain`, `my-name.big-clock`.
- Ids that start with `builtin.` are reserved. You cannot install or remove them.
- `api` must be exactly `1`. A manifest with another number is refused.
- `entry` is a path inside your folder. It cannot start with `/`. It cannot use `..`. The file must exist.

Example background:

```json
{
  "id": "acme.rain",
  "name": "Acme Rain",
  "version": "1.0.0",
  "type": "background",
  "api": 1,
  "entry": "main.js",
  "options": {
    "color": { "type": "color", "default": "#00ff66", "label": "Color" },
    "speed": { "type": "number", "default": 1, "min": 0.1, "max": 5, "label": "Speed" }
  }
}
```

## Options

Each key in `options` is the name of one setting. The settings window is built from these entries, so you write no settings code.

| Field | Meaning |
|---|---|
| `type` | The kind of value. See the table below. |
| `default` | The starting value. Required. It must be valid for the `type`. |
| `label` | The text shown next to the setting. |
| `min`, `max` | Limits for a `number`. Both are optional. |
| `values` | The allowed choices for an `enum`. Required for an `enum`. |

| Kind | Value |
|---|---|
| `string` | Text. |
| `number` | A number. It must be inside `min` and `max` when you set them. |
| `boolean` | `true` or `false`. |
| `color` | Text like `#00ff66`. Six hex digits. Short forms like `#0f6` are not allowed. |
| `enum` | Text that is one of `values`. |
| `file` | Text. It is the path of a file the user chooses. |

## Layouts

A layout is data. It has no code. Put it in the `layout` field of a manifest with `"type": "layout"`.

A layout has `modes`. Each mode is one screen state. Each mode lists regions. Each region holds a list of components. A component is `{ "id": "..." }` and can also carry `options`.

Modes:

- `idle`: required. Nothing needs the user.
- `attention`: required. An agent is waiting for the user.
- `secondary`: optional.

Regions: `top-left`, `top-right`, `center`, `bottom`. Other names are refused. A region you leave out stays empty.

```json
{
  "id": "acme.simple",
  "name": "Simple",
  "version": "1.0.0",
  "type": "layout",
  "api": 1,
  "layout": {
    "modes": {
      "idle": {
        "center": [{ "id": "acme.clock" }],
        "bottom": [{ "id": "acme.cards" }]
      },
      "attention": {
        "bottom": [{ "id": "acme.cards" }]
      }
    }
  }
}
```

## Themes

A theme picks a background and a layout, and sets style values. It needs no entry file.

Two themes are built in: `builtin.default` (Default, on the Matrix background) and `builtin.calm` (Calm, on `builtin.aerials`; it shows Matrix until aerial videos are downloaded).

| Field | Meaning |
|---|---|
| `background` | `{ "id": "...", "options": { ... } }`. The id is required. |
| `layout` | The id of a layout. Required. |
| `style` | An object with style values, such as colors, fonts and sizes. |

A user can replace the theme's background with any installed background: `agent-screensaver background set <id>`. `background set default` goes back to the theme's background. The theme's options for a background still apply when the user picks that same background.

```json
{
  "id": "acme.night",
  "name": "Night",
  "version": "1.0.0",
  "type": "theme",
  "api": 1,
  "theme": {
    "background": { "id": "acme.rain", "options": { "speed": 0.5 } },
    "layout": "acme.simple",
    "style": {}
  }
}
```

## The web API

These are the types an extension is written against. The API version is `1`, the same number as `api` in the manifest.

```ts
/** Version of the extension API. A manifest must declare this exact number. */
export const API_VERSION = 1;

export type Region = "top-left" | "top-right" | "center" | "bottom";
export type Mode = "idle" | "attention" | "secondary";
export type CardKind = "approval" | "running" | "finished" | "unknown" | "no_activity";

/** One agent session. Field names match the service's state.json. */
export interface Card {
  source: string;
  session: string;
  kind: CardKind;
  project?: string;
  title?: string;
  request?: string;
  started_at_ms: number;
  updated_at_ms: number;
  approval_since_ms?: number;
  /** How many background jobs (shells, subagents) the session still runs. Left out when none run or the agent does not say (Codex). */
  background?: number;
  /** The conversation name the agent shows (Claude /rename or its own title, the Codex thread name). Left out when unknown or in privacy mode. */
  name?: string;
  /** Short model name, for example "Opus 5.5" or "gpt-5.6". */
  model?: string;
  /** Git branch checked out in the session's folder. Left out outside a repository and in privacy mode. */
  branch?: string;
  /** How full the context window is, 0 to 100. Left out when the agent does not say. */
  context_percent?: number;
}

/** One usage limit. `resets_at` is in seconds, the other times are in milliseconds. */
export interface Limit {
  source: string;
  window: "5h" | "week" | "plan";
  used_percent: number;
  resets_at?: number;
  updated_at_ms: number;
  /** True when the percent was worked out locally between exact readings. */
  estimated?: boolean;
}

export interface ViewState {
  mode: Mode;
  cards: Card[];
  limits: Limit[];
  nowMs: number;
  serviceOk: boolean;
  display: "main" | "secondary";
}

export interface Ctx {
  options: Readonly<Record<string, unknown>>;
  /**
   * Calls `cb` every frame until you call the returned cancel function. Call
   * it once: calling it again inside `cb` adds a second request each frame.
   * `fps` is the rate you want; leave it out for every display frame. The
   * user's frame ceiling can still lower it.
   */
  requestFrame(cb: (nowMs: number) => void, fps?: number): () => void;
  now(): number;
  display: "main" | "secondary";
}

export interface BackgroundExt {
  start(root: HTMLElement, ctx: Ctx): void;
  stop(): void;
}

export interface ComponentExt {
  mount(root: HTMLElement, ctx: Ctx): void;
  update(state: ViewState): void;
  /** Optional. Called when the same component moves to a placement with other options. */
  setOptions?(options: Readonly<Record<string, unknown>>): void;
  unmount(): void;
}

/** What an entry file passes to `window.agentScreensaver.register(create)`. */
export type CreateExtension = () => BackgroundExt | ComponentExt;
```

A limit is the last reading the service got, and it can be old. Between exact Claude readings the service estimates the Claude limits from local token use (`docs/event-format.md`, "Live Claude limits"); such a limit has `estimated: true`, and the built-in rings add "est." to its caption. The service also works out the reset time of the Claude windows when no exact one is known: the 5h window opens at the first call after the previous window ended and lasts 5 hours, and the week rolls forward from the last exact reset time by whole weeks. An exact `resets_at` (status line, `/usage` cache) always wins. A Claude week that never had an exact reset time stays without one.

The built-in rings never show an old number as the current one. The caption under each ring has two lines: the name ("Claude 5h"), then the reset time, then the age when stale and "est." for an estimate, for example "4h 12m · est.".

- Reset time: more than 24 hours away it is the local day and time, for example "Mon 09:00" (English short weekday, 24-hour clock, no locale data). Within 24 hours (24 hours exactly included) it is a countdown to the minute, rounded down: "4h 12m", under an hour "47m", under a minute "under 1m". A limit with no `resets_at` shows no reset time.
- When `nowMs` is at or after `resets_at`, the window has reset since the reading. The ring is empty and its caption says "Reset" until a newer reading arrives.
- When the reading is older than 30 minutes (5h window) or 6 hours (week and plan windows), the ring is dimmed and its caption adds the age, for example "20h ago".
- The runtime gives every component a new `nowMs` once a minute, and whenever the state changes, so countdowns move without a new state file. Components must take the time from `state.nowMs`, never from the clock.

Components that show limits should follow the same rule.

The `entry` file is one classic script. It must not use `import` or `export`. If you use a bundler, bundle everything into this one file. The script calls `window.agentScreensaver.register(create)` with a `create` function. A background returns an object with `start` and `stop`. A component returns an object with `mount`, `update` and `unmount`. It can also have a `setOptions` method. It is called when the same component moves to a placement with other options. Without it, the component is mounted again. The types above describe each method and the data it receives.

### When macOS shows its lock screen

If the Mac asks for a password after the screen saver starts, macOS draws its lock screen (its clock and the login field) over the running saver when the user moves the mouse or presses a key. While it shows, the page fades out every component and the "service not running" line. The background keeps playing. When the lock screen goes away and the saver still runs, the components come back.

Your extension does not need to do anything. A component stays mounted and keeps getting `update` calls while it is hidden. Nothing tells it that it is hidden.

## The task row's agent marks

Each card of `builtin.task-row` shows the mark and the name of its agent, from the agent extension ([agents.md](agents.md)). The project ships no company logos: the built-in marks are letter tiles of our own. You can put your own icon for any agent in `marks/<source>.svg` in the data folder; see "The mark" in [agents.md](agents.md). A card whose source has no agent extension shows a neutral dot and the source name.

## The Apple aerials background

`builtin.aerials` plays Apple aerial videos that macOS has already downloaded. It has three options: `group` (`cities`, `landscapes`, `space` or `underwater`; default `cities`), `fit` and `dim`, the same as `builtin.video`.

The service gives the background one more option, `files`: a list of `file://` URLs of the videos to play. You cannot set it yourself. The background plays the list in a shuffled order and crossfades from one video to the next. When no video of the group is downloaded, the service shows `builtin.matrix` instead, with the user's Matrix options. `option set builtin.aerials group <group>` then prints how to get the videos: pick the group once in System Settings → Screen Saver, and macOS downloads them.

A theme uses it like any other background: `"background": { "id": "builtin.aerials", "options": { "group": "space" } }`. A user can also pick it without a theme: `agent-screensaver background set builtin.aerials`.

## Install, list and remove

Install from a folder or a `.zip` file. The service copies the extension into the extensions folder. If you install the same `id` again, the new version replaces the old one.

An install is refused when:

- The manifest is not valid.
- The `id` starts with `builtin.`.
- A `.zip` has more than 1000 entries, or is more than 50 MiB when unpacked.
- A `.zip` has an entry with an absolute path or a `..` in its path.
- A `.zip` or a folder holds a symlink.

When an install is refused, nothing is left behind.

When the extensions are listed, an extension is skipped if its manifest is not valid, or if its folder name is not the same as its `id`. An install always names the folder after the `id`.
