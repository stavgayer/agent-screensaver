// A background that reacts to the agents: a dark base that changes tint with the view mode.
// The file is a classic script. It has no import and no export.
(function () {
  var TINTS = { idle: "#05080a", attention: "#2a1a05", secondary: "#05100a" };

  function create() {
    var layer;

    return {
      start: function (root) {
        layer = document.createElement("div");
        layer.style.cssText = "width:100%;height:100%;transition:background-color 600ms ease";
        layer.style.backgroundColor = TINTS.idle;
        root.appendChild(layer);
      },
      // The runtime calls update after start and on every new view state.
      update: function (state) {
        if (layer) layer.style.backgroundColor = TINTS[state.mode] || TINTS.idle;
      },
      stop: function () {
        if (layer) layer.remove();
        layer = undefined;
      }
    };
  }

  window.agentScreensaver.register(create);
})();
