#!/bin/bash
# Run as the owner, OUTSIDE the sandbox. Seeds the three candidate data dirs and
# the aerial video checks (A3: video outside DATA_DIR, A4: APFS clone inside it).
#   probe/seed.sh          seed
#   probe/seed.sh --clean  delete every probe/ folder it created (including the clone)
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

# Aerials macOS already downloaded. The original is only ever read, never modified.
AERIALS="$HOME/Library/Application Support/com.apple.wallpaper/aerials/videos"
DATA_PROBE="${CANDIDATES[0]}/probe"

if [ "${1:-}" = "--clean" ]; then
  for base in "${CANDIDATES[@]}"; do
    rm -rf "$base/probe" && echo "removed $base/probe"
  done
  exit 0
fi

# The probe opens the first .mov by sorted name; pick the same one (C locale = byte order).
first_mov() { LC_ALL=C ls "$AERIALS" 2>/dev/null | LC_ALL=C sort | grep '\.mov$' | head -n 1; }

VIDEO_HTML='<!doctype html>
<html><body style="margin:0;background:#000">
<video id="v" muted autoplay playsinline loop style="width:100%"></video>
<script src="video-path.js"></script>
<script>document.getElementById("v").src = "file://" + encodeURI(window.videoPath);</script>
</body></html>'

CLONE_HTML='<!doctype html>
<html><body style="margin:0;background:#000">
<video src="clone.mov" muted autoplay playsinline loop style="width:100%"></video>
</body></html>'

free_kb() { df -k "$1" | awk 'NR==2 {print $4}'; }

seed_video() {
  local mov original
  mov=$(first_mov)
  if [ -z "$mov" ]; then
    echo "aerials: no .mov in $AERIALS (A3/A4 will report it)"
    return
  fi
  original="$AERIALS/$mov"
  printf '%s\n' "$VIDEO_HTML" > "$DATA_PROBE/video.html"
  printf '%s\n' "$CLONE_HTML" > "$DATA_PROBE/video-clone.html"
  printf 'window.videoPath = "%s";\n' "$original" > "$DATA_PROBE/video-path.js"

  local before after
  before=$(free_kb "$DATA_PROBE")
  rm -f "$DATA_PROBE/clone.mov"
  if cp -c "$original" "$DATA_PROBE/clone.mov"; then
    after=$(free_kb "$DATA_PROBE")
    echo "aerials: original $mov = $(du -k "$original" | awk '{print $1}') KB"
    echo "aerials: free space before clone = $before KB, after = $after KB (delta $((before - after)) KB; ~0 means clonefile shared the blocks)"
  else
    echo "aerials: cp -c failed (not APFS?); A4 will report clone.mov missing"
  fi
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

if [ -d "$DATA_PROBE" ]; then seed_video; else echo "aerials: $DATA_PROBE missing, skipped"; fi
