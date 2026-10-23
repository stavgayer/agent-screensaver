// A background: one picture that ships inside the extension folder.
// It draws once, so it costs almost no CPU. It has no animation loop.
(function () {
  // The script's own address, read once while it loads. It is a file:// URL,
  // so a file next to this script has the same folder.
  var base = document.currentScript.src.replace(/[^/]*$/, "");

  function create() {
    var stage;
    return {
      start: function (root, ctx) {
        var dim = typeof ctx.options.dim === "number" ? ctx.options.dim : 0.4;
        stage = document.createElement("div");
        stage.style.cssText = "position:relative;width:100%;height:100%;background:#000";
        var picture = document.createElement("img");
        picture.alt = "";
        picture.style.cssText = "position:absolute;inset:0;width:100%;height:100%;object-fit:cover";
        picture.src = base + "bg.svg";
        // A dark layer keeps the clock and the cards readable.
        var shade = document.createElement("div");
        shade.style.cssText = "position:absolute;inset:0;background:rgba(0,0,0," + dim + ")";
        stage.append(picture, shade);
        root.append(stage);
      },
      stop: function () {
        if (stage) stage.remove();
        stage = undefined;
      }
    };
  }

  window.agentScreensaver.register(create);
})();
