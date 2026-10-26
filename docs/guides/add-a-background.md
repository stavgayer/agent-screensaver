# Add a background

Goal: draw the full-screen background behind the clock and the cards. It can be a canvas animation, a picture or a video.

Samples:

- [examples/acme.drift](examples/acme.drift): a canvas animation with its own frame rate.
- [examples/acme.still](examples/acme.still): a picture that ships in the extension folder.
- [examples/acme.tint](examples/acme.tint): a background that changes tint with the agents' state.

The built-in `builtin.matrix`, `builtin.image` and `builtin.video` use the same API. Read `web/src/builtins/` for more. The API is in [extensions.md](../extensions.md), "The web API".

## 1. Make the folder and the manifest

```
acme.drift/
  extension.json
  main.js
```

```json
{
  "id": "acme.drift",
  "name": "Drift",
  "version": "1.0.0",
  "type": "background",
  "api": 1,
  "entry": "main.js",
  "options": {
    "color": { "type": "color", "default": "#6cc5b9", "label": "Color" },
    "count": { "type": "number", "default": 60, "min": 5, "max": 300, "label": "Dots" },
    "speed": { "type": "number", "default": 1, "min": 0.2, "max": 4, "label": "Speed" },
    "fps": { "type": "number", "default": 20, "min": 1, "max": 60, "label": "Frames per second" }
  }
}
```

The options become the settings form. See [add-a-widget.md](add-a-widget.md) for the option types.

## 2. Write the entry file

The entry is one classic script, with no `import` and no `export`. It calls `window.agentScreensaver.register(create)`. `create` returns an object with `start(root, ctx)` and `stop()`.

```js
(function () {
  function create() {
    var canvas, cancelFrame;
    return {
      start: function (root, ctx) {
        canvas = document.createElement("canvas");
        canvas.style.cssText = "display:block;width:100%;height:100%";
        root.appendChild(canvas);
        var g = canvas.getContext("2d");
        cancelFrame = ctx.requestFrame(function (nowMs) {
          // draw one frame here
        }, 20);
      },
      stop: function () {
        if (cancelFrame) cancelFrame();
        if (canvas) canvas.remove();
      }
    };
  }
  window.agentScreensaver.register(create);
})();
```

`root` is your own `div`, as large as the screen. Size your canvas from `root.clientWidth` and `root.clientHeight`, multiplied by `window.devicePixelRatio`. Listen to `resize` and remove the listener in `stop`. The full file is in `examples/acme.drift/main.js`.

## 3. Choose the frame rate

- `ctx.requestFrame(cb, fps)` calls `cb` at `fps` frames a second at most. Without `fps` it runs on every display frame, up to 240. Ask for the lowest rate that looks good. A slow drift needs 20 or less.
- The user can set a ceiling: `agent-screensaver fps max 30` (or `display` for no ceiling). The runtime uses the lower of your `fps` and the ceiling. You do not read the ceiling.
- Call `requestFrame` once. Calling it again inside `cb` adds a second request every frame.
- Offer an `fps` option, like `builtin.matrix` does, so the user can trade smoothness for battery.
- Use the time in `cb(nowMs)` to move things, not a fixed step. Cap a long gap (the Matrix caps it at 250 ms), so that a pause does not make things jump.

## 4. Keep the cost low

We measured the built-in backgrounds on an Apple M4 Pro, at 1512 x 982 (`docs/decisions/0002-video-cost.md`). These are shares of one core, with the web processes added up:

| Background | CPU | Memory |
|---|---|---|
| Still image | 3.4% | 77 MB |
| Matrix, 24 fps | 5.9% | 213 MB |
| 4K H.264 video, 30 fps | 6.3% | 273 MB |
| 4K HEVC 10-bit video | 7.8% to 8.3% | 433 MB |

Use the Matrix as your budget. A background that costs more than about 6% of one core on a modern Mac needs a reason. We did not measure 1080p video or battery power.

Rules that keep it cheap:

- Draw only when something changes. A still picture needs no frame loop.
- Reuse objects. Do not allocate in every frame.
- Redraw a small area, or fade with a low-alpha fill, instead of clearing a large canvas when the effect allows it.
- Do not use `backdrop-filter`, large blur or shadow on a moving layer.
- Give the background a dark base. The clock and the cards sit on top of it, and the user reads them from far away. `builtin.image` and `builtin.video` add a dark layer with a `dim` option for this.

## 5. Pause when hidden

When the screen saver stops, the saver hides its web view, and the page becomes hidden. The frames you ask for with `ctx.requestFrame` stop by themselves. You do not write code for it.

Anything else must pause itself. A video does. `builtin.video` does it like this:

```js
document.addEventListener("visibilitychange", function () {
  if (document.hidden) video.pause(); else video.play().catch(function () {});
});
```

Remove the listener in `stop`. Free what you hold in `stop` too: `builtin.video` removes the `src` and calls `load()` to free the decoder.

## 6. Use a picture or a video

There are two ways. Pick one.

A file the user chooses. Add an option of type `file`. The user picks the file in the settings window (or `agent-screensaver option set <id> file /path/to/file`). The service copies it into the data folder, because the screen saver can read only that folder. Your option then holds an absolute path. `builtin.image` and `builtin.video` do this, and have an `fit` option (`cover` or `contain`) and a `dim` option. You can use them without writing any code: `agent-screensaver background set builtin.image`. A `file` default of `""` means no file chosen: show black.

A file that ships with your extension. Read the address of your own script once while it loads. A file in the same folder has the same address, with the last part changed:

```js
var base = document.currentScript.src.replace(/[^/]*$/, "");
// later, in start():
picture.src = base + "bg.svg";
```

`examples/acme.still` does this with a picture. Read `document.currentScript` at the top of the script, not inside `create` or `start`: it is empty after the script has loaded.

Pictures load only from files and `data:` addresses. Videos load only from files. The page cannot open a network address.

## 7. React to your agents

A background can follow the agents. Add an optional `update(state)` method next to `start` and `stop`. The runtime calls it after `start`, and again on every new view state, the same way it calls a component's `update`. The state is the `ViewState` described in [extensions.md](../extensions.md). For example, `state.mode` is `idle`, `attention` or `secondary`.

```js
return {
  start: function (root) { /* build your layer */ },
  update: function (state) {
    layer.style.backgroundColor = state.mode === "attention" ? "#2a1a05" : "#05080a";
  },
  stop: function () { /* remove your layer */ }
};
```

Keep `update` cheap. It runs on every state change and once a minute. Store what you need and let your frame loop draw it, instead of drawing inside `update`. If `update` throws, the runtime stops the background, as it does for a component. A background without `update` works as before. The full file is `examples/acme.tint/main.js`.

## 8. Install and try it

1. `agent-screensaver ext install ./acme.drift`
2. `agent-screensaver background set acme.drift`
3. `agent-screensaver background set default` goes back to the theme's background.
4. To make it part of a theme, see [create-a-theme.md](create-a-theme.md).

To test and share: [test-and-share.md](test-and-share.md).
