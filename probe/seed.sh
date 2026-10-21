#!/bin/bash
# Run as the owner, OUTSIDE the sandbox. Seeds the three candidate data dirs.
# No `set -e`: one failing candidate must not stop the others.
set -u

CONTAINER="$HOME/Library/Containers/com.apple.ScreenSaver.Engine.legacyScreenSaver/Data"
CANDIDATES=(
  "$HOME/Library/Application Support/agent-screensaver"
  "/Users/Shared/agent-screensaver"
  "$CONTAINER/Library/Application Support/agent-screensaver"
)

INDEX_HTML='<!doctype html>
<html><body style="margin:0;background:#111;color:#fff;font:bold 22px monospace;padding:8px">
<div>web load OK</div>
<div id="s">sibling script MISSING</div>
<script src="data.js"></script>
<script>
  var ok = !!(window.data && window.data.ok);
  document.getElementById("s").textContent = ok ? "sibling script OK" : "sibling script MISSING";
  try { webkit.messageHandlers.probe.postMessage(ok ? "sibling-ok" : "sibling-missing"); } catch (e) {}
</script>
</body></html>'

seed_one() {
  local dir="$1/probe"
  mkdir -p "$dir" &&
    printf '{"ok":true}\n' > "$dir/state.json" &&
    printf '%s\n' "$INDEX_HTML" > "$dir/index.html" &&
    printf 'window.data = {ok: true};\n' > "$dir/data.js"
}

i=1
for base in "${CANDIDATES[@]}"; do
  if err=$(seed_one "$base" 2>&1); then
    # Candidate 2 must be owner-only (mode 700).
    [ "$i" = 2 ] && chmod 700 "$base" "$base/probe"
    echo "candidate $i: seeded  $base/probe"
  else
    echo "candidate $i: could not write: $err  ($base)"
  fi
  i=$((i + 1))
done
