#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"
ffmpeg -hide_banner -loglevel error -y -i ../media/poke-demo.mp4 \
  -filter_complex 'fps=12,scale=1040:-1:flags=lanczos,split[a][b];[a]palettegen=max_colors=256:stats_mode=diff[p];[b][p]paletteuse=dither=bayer:bayer_scale=4:diff_mode=rectangle' \
  -loop 0 ../media/poke-demo.gif
ffmpeg -hide_banner -loglevel error -y -ss 13 -i ../media/poke-demo.mp4 \
  -frames:v 1 -update 1 ../media/poke-demo-poster.png
