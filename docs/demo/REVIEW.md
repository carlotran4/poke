# Delivery review

A 14-second silent terminal illustration, with real scripted Poke CLI messages
and receipts as its factual source. The native-agent conversation is illustrated,
not footage of autonomous execution. That label is included in the video itself.
No voiceover or soundtrack was requested; silence suits a looping GitHub README.

Hyperframes 0.9.46 checks passed runtime, layout and contrast (all sampled text
passed). Scene/beat sheets and exact request/reply/end frames were inspected.
The 1080p text, clipped containers, message sequence, ending receipts and quoted
commands were checked visually. The final render is inspected again after decode.

A fresh reviewer found no desktop blockers but found the terminal details too
small at phone width. The request/reply are now repeated as 62 px headline text,
so the core exchange survives that reduction. Detailed command/JSON reading is
intended for desktop or the linked full-resolution MP4. Terminal chrome and
metadata deliberately remain smaller than the main reading text.

Retained craft warnings: machine metadata is below the 28 px phone type floor;
joined terminal containers are flagged as 21/28-word prose blocks with short
reading holds. Individual messages and commands reveal separately, the key
message is repeated above the panes, and the video is an overview rather than a
step-by-step CLI tutorial. No layout/contrast errors are waived.

Weakest view: the reduced mobile two-pane layout cannot make every CLI argument
legible. A separate vertical tutorial would improve that, but would be a different
format from this README hero. No stock footage, mock dashboard, music, sound,
avatar or generated human presenter was added.

Export verification: decoded MP4 frames were inspected across the request,
acknowledgment, reply and ending. `ffprobe` reports H.264, 1920×1080, 30 fps,
420 frames, 14.000 seconds. The looping GIF is 1040×585, 168 frames, 14.000 seconds,
about 1.2 MB. Audio inspection confirms no audio stream, intentionally. No sound
was measured or heard. The final lint report has zero errors and zero warnings.
