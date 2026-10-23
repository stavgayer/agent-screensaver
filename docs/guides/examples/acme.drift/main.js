// A background: slow dots that drift up the screen on a canvas.
// The file is a classic script. It has no import and no export.
(function () {
  function number(value, fallback, min, max) {
    return typeof value === "number" && isFinite(value) ? Math.min(max, Math.max(min, value)) : fallback;
  }

  function create() {
    var canvas, cancelFrame, onResize;

    return {
      start: function (root, ctx) {
        var color = typeof ctx.options.color === "string" ? ctx.options.color : "#6cc5b9";
        var count = number(ctx.options.count, 60, 5, 300);
        var speed = number(ctx.options.speed, 1, 0.2, 4);
        var fps = number(ctx.options.fps, 20, 1, 60);

        canvas = document.createElement("canvas");
        canvas.style.cssText = "display:block;width:100%;height:100%";
        root.appendChild(canvas);
        var g = canvas.getContext("2d");
        if (g === null) return;

        var width = 1, height = 1, dots = [];
        function fit() {
          var ratio = window.devicePixelRatio || 1;
          width = root.clientWidth || 1;
          height = root.clientHeight || 1;
          canvas.width = Math.round(width * ratio);
          canvas.height = Math.round(height * ratio);
          g.setTransform(ratio, 0, 0, ratio, 0, 0);
        }
        fit();
        onResize = fit;
        window.addEventListener("resize", onResize);

        for (var i = 0; i < count; i++) {
          dots.push({ x: Math.random(), y: Math.random(), r: 1 + Math.random() * 3, v: 0.01 + Math.random() * 0.03 });
        }

        var last;
        // One request. The runtime calls it at min(fps, the user's ceiling),
        // and stops calling it while the page is hidden.
        cancelFrame = ctx.requestFrame(function (nowMs) {
          var seconds = last === undefined ? 1 / fps : Math.min(0.25, (nowMs - last) / 1000);
          last = nowMs;
          g.fillStyle = "#05080a";
          g.fillRect(0, 0, width, height);
          g.fillStyle = color;
          g.globalAlpha = 0.6;
          for (var d = 0; d < dots.length; d++) {
            var dot = dots[d];
            dot.y -= dot.v * speed * seconds;
            if (dot.y < -0.02) { dot.y = 1.02; dot.x = Math.random(); }
            g.beginPath();
            g.arc(dot.x * width, dot.y * height, dot.r, 0, Math.PI * 2);
            g.fill();
          }
          g.globalAlpha = 1;
        }, fps);
      },
      stop: function () {
        if (cancelFrame) cancelFrame();
        cancelFrame = undefined;
        if (onResize) window.removeEventListener("resize", onResize);
        onResize = undefined;
        if (canvas) canvas.remove();
        canvas = undefined;
      }
    };
  }

  window.agentScreensaver.register(create);
})();
