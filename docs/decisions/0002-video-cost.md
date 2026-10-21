# 0002: Cost of the image and video backgrounds

Status: accepted (2026-10-21)

## Context

Spec section 8 measured the Matrix rain at about 6.5% of one core and said "not measured yet: a video background". `builtin.video` plays a file in a `<video>` element. We needed to know what it costs, and how it compares with the Matrix and with a still image.

## How we measured

- Machine: Apple M4 Pro, macOS 26.6.2, window 1512x982.
- Saver: the real `AgentScreensaver.saver`, run in the saver harness (a normal window outside `legacyScreenSaver`). The harness read a temp data dir with `build/runtime`, `build/extensions` and a `state.json` made by the daemon. The theme was `builtin.default` (Matrix) or a test theme with `builtin.video` or `builtin.image`. The clock showed in every run, so each run has the same screen on top of the background.
- The file was set with `option set`, so the service copied it into `media/builtin.video/file/` and stored the absolute path, as a user's setting would.
- Samples: `top -l 3 -s 10`. The first sample is not a delta, so it is dropped. The numbers are the mean of the next two samples, a 20 s window that starts 10 s after the saver starts.
- Processes counted: the harness, and the three WebKit processes it started (GPU, Networking, WebContent). For HEVC, `VTDecoderXPCService` also started, so it is counted. `WindowServer` is shared by the whole machine and is not counted.
- Memory is `top`'s MEM column (resident size), summed over the same processes. Resident sizes can count shared pages more than once, so a sum can be higher than the real use.

Video samples:

| Sample | Codec | Size | Frame rate | Bit rate | File |
| --- | --- | --- | --- | --- | --- |
| Apple aerial (a copy of one file from the owner's aerials folder) | HEVC Main 10 | 3840x2160 | 239.76 fps in the file | 9.5 Mb/s | 105 MB, 88 s |
| `ffmpeg` test pattern | H.264 High | 3840x2160 | 30 fps | 20 Mb/s | 48 MB, 20 s |

The aerial really has about 240 frames a second (21073 frames in 87.9 s). We did not measure how many frames the player decodes or shows, so we only call it a hard case by its file.

## Results

CPU is a share of one core, all counted processes together.

| Background | Harness | WebKit GPU | WebContent | VT decoder | CPU total | Memory total |
| --- | --- | --- | --- | --- | --- | --- |
| Still image (4K PNG) | 1.65% | 0.20% | 1.55% | n/a | 3.4% | 77 MB |
| Matrix, 24 fps | 2.20% | 1.55% | 2.10% | n/a | 5.9% | 213 MB |
| Video, 4K H.264, 30 fps | 2.05% | 2.30% | 1.90% | n/a | 6.3% | 273 MB |
| Video, 4K HEVC 10-bit aerial (run 1) | 1.75% | 2.95% | 1.65% | 1.40% | 7.8% | 433 MB |
| Video, 4K HEVC 10-bit aerial (run 2) | 1.95% | 3.05% | 1.75% | 1.50% | 8.3% | 433 MB |

Memory per process (MB, mean of the same two samples; one run each, the HEVC row is run 2, and run 1 gave the same 346 for the decoder):

| Background | Harness | WebKit GPU | Networking | WebContent | VT decoder |
| --- | --- | --- | --- | --- | --- |
| Still image | 21.0 | 14.0 | 3.7 | 38.0 | n/a |
| Matrix | 20.5 | 135.0 | 3.4 | 54.0 | n/a |
| Video, H.264 | 22.0 | 211.5 | 0.0 | 39.5 | n/a |
| Video, HEVC | 21.5 | 24.0 | 0.0 | 41.5 | 346.0 |

Notes on the numbers:

- The Matrix run gives 5.9%, close to the spike's 6.5%. The method is consistent with the spike.
- The still image is the lowest row: about 3.4%. We think this is the clock, the state timer and the web view, and that the image adds almost nothing because it is drawn once with no animation loop. This is an inference: we did not run a plain black background to check.
- H.264 is decoded inside the WebKit GPU process (211 MB there). HEVC 10-bit starts a separate `VTDecoderXPCService` that holds about 346 MB of frames.
- WindowServer was at 51% to 57% in every run, even with the still image. A build was running on the machine (load average 17 to 62 during the runs), so the machine was not quiet. Treat the numbers as good to about one percentage point. Only the two HEVC runs were repeated, and they agree within 0.5 points. The other rows are single runs.

## Decision

- Ship both backgrounds as built. A 4K video costs about 6% to 8% of one core. That is 0 to 2.5 points more than the Matrix, and the worst case we tried (240 fps HEVC 10-bit) is under 9%.
- Memory is the larger cost: 270 to 430 MB for 4K video, against 213 MB for the Matrix. The decoder probably holds the frames, so we expect video size to matter more than length. We did not test this.
- 1080p was not measured. We expect it to cost less than 4K, but the docs make no claim about it.
- The video is always muted and pauses when the page is hidden. There is no option for sound.
- The image background has no animation loop. It needs no CPU beyond the screen's own clock.

## Not measured

- 1080p files and battery power.
- The pause on `visibilitychange` inside the real `legacyScreenSaver` host. We could not make the host hide the page, so only the code path is written; it is not observed.
- Playback of an APFS clone of an aerial was not measured separately. These runs played a real copy under the data dir; a clone holds the same bytes, so we expect the same cost. Sandbox access to the aerials is settled in 0001.
