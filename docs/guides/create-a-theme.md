# Create a theme

Goal: give people one choice that sets the background, the layout and the colors together.

A theme is data. It has no code and no entry file. Sample: [examples/acme.dusk](examples/acme.dusk). The built-in themes are `extensions/builtin.default/extension.json` (Default: Matrix rain, green) and `extensions/builtin.calm/extension.json` (Calm: Apple aerial videos, soft teal).

Before you start: the background and the layout you want are installed. Use built-in ones, or your own: [add-a-background.md](add-a-background.md), [change-the-layout.md](change-the-layout.md).

## 1. Make the folder

```
acme.dusk/
  extension.json
```

```json
{
  "id": "acme.dusk",
  "name": "Dusk",
  "version": "1.0.0",
  "type": "theme",
  "api": 1,
  "theme": {
    "background": { "id": "acme.drift", "options": { "color": "#ff9a4d", "speed": 0.6 } },
    "layout": "acme.compact",
    "style": {
      "--accent": "#ff9a4d",
      "--attention": "#ffd166",
      "--done": "#9ad1ff",
      "--card-opacity": "0.86"
    }
  }
}
```

## 2. Fill the three parts

| Field | Meaning |
|---|---|
| `background` | `{ "id": "...", "options": { ... } }`. The `id` is required. The `options` are optional values for that background. Only options the background declares, with valid values, are used. |
| `layout` | The id of a layout. Required. |
| `style` | Style values, as an object. |

The `style` keys that start with `--` are set as CSS variables on the page. Other keys are ignored. The built-in parts read these:

| Variable | Used for |
|---|---|
| `--accent` | The main color: working state, rings, clock tint. |
| `--attention` | A waiting agent: the approval card and the "Check the terminal" state. |
| `--done` | A finished task. |
| `--card-opacity` | How solid the cards are. |

Your own widgets can read any variable you set. Give each a fallback value in your CSS.

## 3. Thumbnail and tile

A theme has no thumbnail field. The settings window draws a small tile for each theme: a black screen with a clock in your `--accent` (and a dusk sky when the theme uses `builtin.aerials`). Set `--accent` so the tile shows your color. The live preview above the tiles shows the real result.

## 4. Install and use it

1. `agent-screensaver ext install ./acme.dusk`
2. `agent-screensaver theme set acme.dusk`
3. `agent-screensaver theme list` shows every theme. A `*` marks the one in use.

The theme is picked up by the running service. You do not restart anything.

A user can still change the background without leaving your theme: `agent-screensaver background set <id>` replaces the theme's background. `background set default` goes back to yours. When they choose the same background your theme uses, your theme's `options` for it still apply. The user's own option values win over the theme's.

## What happens when a part is missing

The service never shows a black screen for a broken theme:

- If the theme is not installed, it uses `builtin.default`.
- If the background is not installed, it uses the background of `builtin.default`.
- If the layout is not installed, it uses the layout of `builtin.default`.

Install checks only that the theme has a background id and a layout id. It does not check that they exist. Test your theme after you install it, with every part in place.

To test and share: [test-and-share.md](test-and-share.md).
