#!/usr/bin/env bash
# Turns the frames from framegrab into the README GIF and, if asked, a video.
# usage: frames-to-gif.sh <frames dir> <scene start ms> <from s> <to s> <out.gif> [out.mp4]
#
# <scene start ms> is the wall-clock time the demo scene started (the state
# writer prints it). Frames are named by their wall-clock ms, so each 1/12 s
# step from <from s> to <to s> of scene time takes the last frame at or
# before it. A late frame repeats the one before it instead of making the
# clip skip. Needs ffmpeg.
set -euo pipefail

frames=$1 start=$2 from=$3 to=$4 gif=$5 mp4=${6:-}
fps=12
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

for file in "$frames"/*.png; do
  name=${file##*/}
  echo "${name%.png}"
done | sort -n |
  awk -v start="$start" -v from="$from" -v to="$to" -v fps="$fps" \
      -v src="$frames" -v dst="$work" '
    { ms[NR] = $1 }
    END {
      i = 1; n = 0
      for (t = from * 1000; t < to * 1000; t += 1000 / fps) {
        while (i < NR && ms[i + 1] - start <= t) i++
        printf "ln -s \"%s/%s.png\" \"%s/%05d.png\"\n", src, ms[i], dst, n++
      }
    }' | sh

# 160 colors and an ordered dither keep the GIF under 8 MB at 960 wide.
ffmpeg -loglevel error -y -framerate "$fps" -i "$work/%05d.png" \
  -vf "scale=960:-1:flags=lanczos,split[a][b];[a]palettegen=max_colors=160:stats_mode=diff[p];[b][p]paletteuse=dither=bayer:bayer_scale=5:diff_mode=rectangle" \
  -loop 0 "$gif"

if [ -n "$mp4" ]; then
  ffmpeg -loglevel error -y -framerate "$fps" -i "$work/%05d.png" \
    -vf "scale=1280:-2:flags=lanczos" -c:v libx264 -crf 20 -preset slow \
    -pix_fmt yuv420p -movflags +faststart "$mp4"
fi
