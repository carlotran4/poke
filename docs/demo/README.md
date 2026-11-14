# Poke README video

An editable [Hyperframes](https://github.com/heygen-com/hyperframes) project for the
14-second Codex/Pi hero in the main README. It renders HTML/GSAP into a 1080p MP4;
the smaller GIF is the inline GitHub preview.

The agent UIs are illustrations. `assets/exchange.json` contains the actual
scripted Poke CLI messages and receipts used to write the scene. No autonomous
model execution or schema review is represented as recorded footage.

## Edit and render

Requires Node 22+, FFmpeg, and Hyperframes' supported Chrome installation.
The authoring dependencies are pinned separately from Poke's Go runtime.

```sh
cd docs/demo
npm ci
npm run check
npm run preview
# Edit the scene/timeline in the preview or in compositions/conversation.html.
npm run render
./encode.sh
```

The temporary preview can be stopped with `npx hyperframes preview --stop`.
`index.html` mounts the scene; `SPEC.md` records the beats and `REVIEW.md` records
visual checks and intentional tradeoffs. The main message text is enlarged above
the two panes so the exchange still reads at reduced GitHub/mobile widths.

Outputs:

- `../media/poke-demo.mp4` — 1920×1080, 30 fps, 14 seconds, silent.
- `../media/poke-demo.gif` — 1040 px wide, 12 fps, looping README preview.
- `../media/poke-demo-poster.png` — the final state at 13 seconds.

No service, credentials, model account or video-generation API is needed to render.
Hyperframes may fetch its bundled font families on the first render. The registry's
terminal primitives were consulted before authoring the custom two-pane scene.
