// Runs inside the settings window's preview frame (runtime/preview.html),
// next to the real runtime. The settings page cannot reach into a file://
// frame, so it posts the state and this hands it to the runtime.
window.addEventListener("message", (event: MessageEvent) => {
  if (event.source !== window.parent) return;
  const data = event.data as { type?: unknown; state?: unknown } | null;
  if (data?.type !== "agent-screensaver-state" || typeof data.state !== "string") return;
  window.agentScreensaver.setState(data.state, "main");
});
