# Add a widget

Goal: draw a new piece of the screen, such as a counter, a quote or a status badge. In the manifest this is a component.

Sample: [examples/acme.agent-count](examples/acme.agent-count). It shows how many agents are working. Full API: [extensions.md](../extensions.md), "The web API", and `web/src/api.ts`.

Before you start: you know basic JavaScript and CSS. You do not need a bundler.

## 1. Make the folder

```
acme.agent-count/
  extension.json
  main.js
```

## 2. Write the manifest

```json
{
  "id": "acme.agent-count",
  "name": "Agent count",
  "version": "1.0.0",
  "type": "component",
  "api": 1,
  "entry": "main.js",
  "options": {
    "label": { "type": "string", "default": "agents working", "label": "Label" },
    "size": { "type": "enum", "default": "large", "values": ["large", "small"], "label": "Size" },
    "show_dot": { "type": "boolean", "default": true, "label": "Show the dot" }
  }
}
```

`options` become the settings form, so you write no settings code. Each option has a `type` (`string`, `number`, `boolean`, `color`, `enum` or `file`) and a `default` that is valid for that type. A `number` can have `min` and `max`. An `enum` needs `values`. A `color` is `#rrggbb` (six digits). The `entry` is a path inside the folder.

## 3. Write the entry file

The entry is one classic script. It must not use `import` or `export`. If you use a bundler, bundle everything into this one file. The script calls `window.agentScreensaver.register(create)`. `create` returns an object with `mount`, `update`, `unmount`, and, if you want, `setOptions`.

```js
(function () {
  function create() {
    var number;
    return {
      mount: function (root, ctx) {
        number = document.createElement("div");
        root.append(number);
      },
      update: function (state) {
        var running = state.cards.filter(function (c) { return c.kind === "running"; }).length;
        number.textContent = String(running);
      },
      unmount: function () { number.remove(); }
    };
  }
  window.agentScreensaver.register(create);
})();
```

- `mount(root, ctx)` runs once. `root` is your own `div`. Put all your elements, and your `<style>`, inside it.
- `update(state)` runs on every state change and once a minute. `state` has `mode`, `cards`, `agents`, `agentOrder`, `limits`, `nowMs`, `serviceOk` and `display`.
- `setOptions(options)` is optional. The runtime calls it when your component moves to a place with other options. Without it, the runtime mounts your component again.
- `unmount()` runs when the component goes away. Remove timers and listeners here.

What `ctx` gives you:

| Member | Meaning |
|---|---|
| `ctx.options` | The option values: your defaults, then the user's values, then the options of the place in the layout. |
| `ctx.requestFrame(cb, fps)` | Calls `cb` at the rate you ask for, at most. It returns a cancel function. Call it once. |
| `ctx.now()` | The time in milliseconds. |
| `ctx.display` | `main` or `secondary`. |

## 4. Read the data you get

- A card has `source`, `session`, `kind` (`approval`, `running`, `finished`, `unknown`, `no_activity`), and these when known: `project`, `title`, `request`, `name`, `model`, `branch`, `context_percent`, `background`.
- Use `state.agents[card.source]` for the agent's `name` and `mark_url`. A card from a source with no agent extension has no entry there.
- A limit has `source`, `window` (`5h`, `week`, `plan`), `used_percent`, and `resets_at` in seconds.
- For countdowns, use `state.nowMs`. Never read the clock yourself. The runtime gives you a new `nowMs` once a minute.
- Card text is data. Set it with `textContent`, never `innerHTML`.

## 5. Follow the rules of the screen

Theme variables. A theme sets these on the page. Use them so your widget follows the theme:

| Variable | Use |
|---|---|
| `--accent` | The main color. |
| `--attention` | Something waits for the user. |
| `--done` | A finished task. |
| `--card-opacity` | The fill of a card. |
| `--fg`, `--muted`, `--bg` | Text, quiet text, page. |

Give each a fallback: `var(--accent, #00e07a)`.

Size. People read this screen from far away. Use `vmin` for sizes, not `px`. Write a text shadow or a dark fill so text stays readable over any background.

Performance:

- Write to the page only when a value changes (`if (el.textContent !== text) ...`). `update` runs often.
- Use `ctx.requestFrame(cb, fps)` for animation, with the lowest `fps` that looks good. A clock needs 1.
- Animate only `opacity` and `transform`.
- Do not use `setInterval` for animation. The runtime stops frames when the page is hidden. Your own timer is not under its control.

Reduced motion. Add a `prefers-reduced-motion: reduce` rule that stops your CSS animations:

```css
@media (prefers-reduced-motion: reduce) { .dot { animation: none; } }
```

Privacy mode. When the user turns privacy on, the service clears `title`, `request`, `name` and `branch` from every card before your code sees it. `project` and `model` stay. Handle a missing `title`. Do not try to get the text from anywhere else.

Failures. If your `create`, `mount` or `update` throws, the runtime stops your extension and logs the error. Other extensions keep running. It is not tried again until the theme changes.

No network. The page blocks every connection (`connect-src 'none'`). Scripts, styles, images, media and fonts load from files only.

## 6. Install and place it

1. `agent-screensaver ext install ./acme.agent-count`
2. A component shows only where a layout puts it. Use it in a layout: see [change-the-layout.md](change-the-layout.md).
3. Change an option from the command line: `agent-screensaver option set acme.agent-count label "agents busy"`. Reset with `agent-screensaver option reset acme.agent-count`.

To change your code, install the folder again. The service reads an extension again only when the extensions folder changes, and an install replaces the old folder.

Then test and share it: [test-and-share.md](test-and-share.md).
