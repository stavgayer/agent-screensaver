# Change the layout

Goal: choose where the clock, the cards and your own widgets go on the screen.

A layout is data. It has no code. Sample: [examples/acme.compact](examples/acme.compact). The built-in layout is `extensions/builtin.spotlight/extension.json`. Reference: [extensions.md](../extensions.md#layouts).

Before you start: the components you want to place are installed (`agent-screensaver ext list`). The built-in ones are `builtin.clock`, `builtin.limit-rings`, `builtin.task-row` and `builtin.approval-card`, and the Chrome look of each: `builtin.chrome-clock`, `builtin.chrome-rings`, `builtin.chrome-tasks` and `builtin.chrome-approval`. To write your own, see [add-a-widget.md](add-a-widget.md).

## The screen

There are four regions: `top-left`, `top-right`, `center` and `bottom`. A region you leave out stays empty. A region holds a list of components, in order, from first to last.

There are three modes. Each mode is a full list of regions:

| Mode | When | Required |
|---|---|---|
| `idle` | No agent waits for the user. | yes |
| `attention` | At least one agent waits for approval. | yes |
| `secondary` | The screen is on a second display. A second display never switches to attention. | no |

If your layout has no `secondary` mode, the runtime tries to show a large `builtin.clock` on a second display. That works only when your layout uses `builtin.clock` somewhere else. Add a `secondary` mode to be sure.

## Where the built-ins go

`builtin.spotlight` is the default layout:

| Region | Idle | Attention |
|---|---|---|
| `top-left` | | clock, small |
| `top-right` | limit rings | limit rings |
| `center` | clock, large | approval card |
| `bottom` | task row | task row |

## 1. Make the folder

```
acme.compact/
  extension.json
```

```json
{
  "id": "acme.compact",
  "name": "Compact",
  "version": "1.0.0",
  "type": "layout",
  "api": 1,
  "layout": {
    "modes": {
      "idle": {
        "top-left": [{ "id": "builtin.clock", "options": { "size": "small" } }],
        "center": [{ "id": "acme.agent-count" }],
        "bottom": [{ "id": "builtin.task-row" }]
      },
      "attention": {
        "top-left": [{ "id": "builtin.clock", "options": { "size": "small" } }],
        "top-right": [{ "id": "acme.agent-count", "options": { "size": "small", "label": "working" } }],
        "center": [{ "id": "builtin.approval-card" }],
        "bottom": [{ "id": "builtin.task-row" }]
      },
      "secondary": {
        "center": [{ "id": "builtin.clock", "options": { "size": "large" } }]
      }
    }
  }
}
```

A component is `{ "id": "..." }`. It can also have `options`. These are options of that one place. They are on top of the user's own values for that component. In the sample, the count shows "working" in the corner and the default label in the center.

## 2. Move a component

To move a component, take its entry out of one region and put it in another. Do it in both modes if you want it in both.

Do not forget the `attention` mode. It must hold what the user needs to see: the `builtin.approval-card` (or a component of your own that shows the waiting card). A layout without it hides the request.

The runtime keeps a component that has the same `id` in both modes. It moves the same component to the new place with a short animation. Only the options that change reach `setOptions`. A component without `setOptions` is mounted again.

## 3. Install and use it

A layout is used by a theme. To try it:

1. `agent-screensaver ext install ./acme.compact`
2. Make a theme that uses it (see [create-a-theme.md](create-a-theme.md)), then `agent-screensaver theme set <theme id>`.

There is no command to pick a layout alone. The theme names it.

## What the service checks

At install, the service refuses a layout that:

- has no `idle` or no `attention` mode: `layout: missing mode "attention"`
- has a mode with another name: `unknown mode`
- uses a region with another name: `layout: unknown region "left" in mode "idle"`
- has an empty component `id`
- has a field other than `modes`

The service does not check, at install, that the components exist. When it builds the screen, it leaves out:

- a component that is not installed, or is not a component,
- an option the component does not declare,
- an option value that is not valid for its type.

If the theme names a layout that is not installed, the service uses the layout of `builtin.default`.

To test and share: [test-and-share.md](test-and-share.md).
