// A component: shows how many agents are working now.
// The file is a classic script. It has no import and no export.
(function () {
  var STYLE =
    ".count { text-align: center; color: var(--fg); font-variant-numeric: tabular-nums;" +
    " text-shadow: 0 0 2vmin #000, 0 0 0.6vmin #000; }" +
    ".count .number { font-size: 14vmin; font-weight: 300; line-height: 1; }" +
    ".count .label { font-size: 2.6vmin; margin-top: 1vmin; color: var(--muted); }" +
    ".count.small .number { font-size: 5vmin; }" +
    ".count.small .label { font-size: 1.8vmin; }" +
    ".count .dot { display: inline-block; width: 1.6vmin; height: 1.6vmin; border-radius: 50%;" +
    " margin-right: 1vmin; background: var(--accent, #00e07a); animation: breathe 2.4s ease-in-out infinite alternate; }" +
    ".count .dot[hidden] { display: none; }" +
    "@keyframes breathe { from { opacity: 1; } to { opacity: 0.35; } }" +
    "@media (prefers-reduced-motion: reduce) { .count .dot { animation: none; } }";

  function create() {
    var box, number, text, dot;

    function applyOptions(options) {
      box.classList.toggle("small", options.size === "small");
      text.textContent = String(options.label);
      dot.hidden = options.show_dot === false;
    }

    return {
      mount: function (root, ctx) {
        var style = document.createElement("style");
        style.textContent = STYLE;
        box = document.createElement("div");
        box.className = "count";
        number = document.createElement("div");
        number.className = "number";
        var label = document.createElement("div");
        label.className = "label";
        dot = document.createElement("span");
        dot.className = "dot";
        text = document.createElement("span");
        label.append(dot, text);
        box.append(number, label);
        root.append(style, box);
        applyOptions(ctx.options);
      },
      update: function (state) {
        var working = state.cards.filter(function (card) { return card.kind === "running"; }).length;
        var shown = String(working);
        if (number.textContent !== shown) number.textContent = shown; // write only on change
      },
      setOptions: applyOptions,
      unmount: function () {
        if (box) box.remove();
        box = number = text = dot = undefined;
      }
    };
  }

  window.agentScreensaver.register(create);
})();
