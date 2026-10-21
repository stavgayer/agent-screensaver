# Writing extensions

agent-screensaver is built from extensions. The Matrix background, the clock, the limit rings and the task cards are all extensions. They use the same format that you use. If you can write one, you can replace any of them.

This page tells you how to write an extension. It only describes what works today.

## The five types

| Type | What it does | Has a manifest |
|---|---|---|
| Background | Draws the full-screen background. | yes |
| Component | Draws one piece of the screen, such as a clock or a card row. | yes |
| Layout | Says where components go on the screen. It is data only. | yes |
| Theme | Picks a background, a layout and style values. | yes |
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
| `type` | string | yes | `background`, `component`, `layout` or `theme`. |
| `api` | number | yes | The API version you need. It must be `1`. |
| `entry` | string | background, component | The JavaScript file to load. |
| `options` | object | no | Settings people can change. See below. |
| `layout` | object | layout | The layout. See below. |
| `theme` | object | theme | The theme. See below. |

Rules:

- The `id` uses lowercase letters, digits and dashes. It has at least one dot. Examples: `acme.rain`, `my-name.big-clock`.
- Ids that start with `builtin.` belong to the parts that ship with agent-screensaver. You cannot install or remove them.
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
        "center": [{ "id": "builtin.clock" }],
        "bottom": [{ "id": "builtin.cards" }]
      },
      "attention": {
        "bottom": [{ "id": "builtin.cards" }]
      }
    }
  }
}
```

## Themes

A theme picks a background and a layout, and sets style values. It needs no entry file.

| Field | Meaning |
|---|---|
| `background` | `{ "id": "...", "options": { ... } }`. The id is required. |
| `layout` | The id of a layout. Required. |
| `style` | An object with style values, such as colors, fonts and sizes. |

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

This section will be filled in with Task 2.1. The web API is what your `entry` file can call. Until then, do not rely on any API.

## Install, list and remove

Install from a folder or a `.zip` file. The service copies the extension into the extensions folder. If you install the same `id` again, the new version replaces the old one.

An install is refused when:

- The manifest is not valid.
- The `id` starts with `builtin.`.
- A `.zip` has more than 1000 entries, or is more than 50 MiB when unpacked.
- A `.zip` has an entry with an absolute path or a `..` in its path.
- A `.zip` or a folder holds a symlink.

When an install is refused, nothing is left behind.

An extension that is not valid is skipped when the extensions are listed.

## Safety rules

The screen saver runs while the Mac is locked. So outside code is limited:

- Web extensions have no network access. The Content Security Policy blocks it.
- Web extensions have no file access. They see only the state the service gives them.
- If an extension throws an error or freezes, the saver drops it and shows the default for that slot.
- The runtime enforces the user's `max_fps` limit for every background that draws through the API, including yours.
- A layout has no code, so it cannot run anything.
- The Go service never loads outside code. A source is a separate program that sends events.
