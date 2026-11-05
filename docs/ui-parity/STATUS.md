# Playing-page parity

## Checkout

Branch `fix/player-ui-parity`, taken from `main` at `106cd3c` (`Rewrite README for listeners`).

`main` matched `origin/main`. The working tree was clean. No local WIP on remote; the user’s ~32 uncommitted files may still be only on their PC.

Nothing from `D:\仓库集合\mygo-cloudmusic` was available here unless it had already been pushed. No commit was invented for that work. No force-push.

`本地开发说明-改代码看这里.md` on this `main` names `internal/app/ui_stage.go`, `ui_stage_styles.go`, and settings `LyricsStage`, `StageStyle`, `StageSparks`, `Fog`, and `Smoke`. Those files and fields are not in the Go tree at `106cd3c`. `ui_nowplaying.go` is 582 lines, not the 650 lines that note claims. A comment on `app.npPane` still says `paneStage`, and that constant is gone. If the uncommitted Windows tree adds a stage view, it is not in this branch.

## What the playing page does now

The page that opens with Ctrl+L is `nowPlayingView` (`internal/app/ui_nowplaying.go`) and `immersive` (`internal/app/ui_immersive.go`).

- The cover is scaled to cover the whole window (proportions kept, edges cropped). It is not a centered square card.
- Wider than height × 1.15, the same picture stays sharp on the left and eases into a blurred copy between 42% and 72% of the width. Square and tall windows stay sharp across.
- The blur fed to that mix is a 48×48 image: shrink, raise saturation, three box blurs of radius 5 (`blurImage` in `internal/app/artwork.go`). Scaled up, it can look soft and banded.
- Under that, `backdrop` (`internal/app/backdrop.go`) paints a deep gradient of the cover’s main colour, lighter at the top, plus the blurred cover at 10% opacity, into a canvas an eighth of the window across. The full-bleed cover is drawn over it, so the gradient shows only where the cover does not. Comments in `backdrop.go` describe this gradient as matching NetEase’s player; the picture on top is a later full-bleed poster.
- Title, artist, and the current lyric line are centered just above the bottom bar. The bar (progress, a 44px thumbnail, transport, quality, volume) hides after 2.75s idle. Lyrics sit in a right column when the window is at least 1120px wide and at least 1.3 times as wide as it is tall; otherwise they are a centered panel.
- Opening and closing spring the whole layer from 60px below (`npSpring`, 450ms, bounce 0.08).

`npCover` in `internal/app/ui_cover.go` still implements a square cover that tilts, glares, and flings to the next song. No caller builds it. A comparison that expects that card will not see it on this tree.

The foot of the normal pages (`playerBar` in `internal/app/ui_player.go`) is a separate bar, height 84. This pass does not restyle it.

Likely places a paired screenshot will disagree, to confirm rather than to fix yet:

1. Official art is often a sharp square over a blurred full-window field. Here the sharp (cropped) cover is the field, and blur is only the right-hand mix on wide windows.
2. Blur radius and resolution: 48×48 and a short box blur, against whatever the official client blurs on the GPU.
3. Motion: no disc spin and no pointer tilt on the page that actually shows. The unused `npCover` path is the one with tilt.
4. Text sits low and centered instead of in a column beside a card. Lyric layout changes at the thresholds above, so 16:9 and 1:1 are different structures, not the same layout scaled.

No visual constants were changed in this pass. A restyle needs the official stills.

## Screenshots from this VM

The official NetEase Windows client does not run here. This app was not logged in and was not given a live song.

`ui.NewTester` drew the playing page without a window. `TestNowPlayingAspectFrames` was run with `NM_FRAMES` set to `docs/ui-parity/frames/mygo` (five times). The cover is synthetic: a 512×512 wash, a disc, and a red square at the cover’s top left. The PNGs are this app only.

What those frames show, which matches the code above:

- `16x9-1280x720.png` and `16x9-1920x1080.png`: the disc’s left edge stays sharp and its right side dissolves into the blurred wash. The red square is cropped off the top.
- `4x3-1024x768.png` and `default-1180x780.png`: same side blur. The red square is cut by the top edge.
- `1x1-800x800.png` and `1x1-1080x1080.png`: the red square is fully in frame, and the disc stays sharp all the way around. No side blur.
- `9x16-720x1280.png`: the red square is cropped off the side. The disc stays sharp.
- `lyrics-16x9-1280x720.png`: the lyric line sits on the right, over the blur.
- `lyrics-1x1-800x800.png`: the lyric line sits in a centered dark panel.
- The bottom bar’s small cover is the same fixture. Controls are the awake state. Nothing here is a recording of the open or close spring.

Left-edge stair-steps, where the disc is sharp, come from drawing a 512×512 fixture into a larger window. A real 1536 cover would be smoother there. The soft right side is the 48×48 blur, not that upscale.

## Tests on this VM

`NM_FRAMES="$PWD/docs/ui-parity/frames/mygo" go test ./internal/app/ -run TestNowPlayingAspectFrames -count=5` passed.

`go test ./internal/...` then failed in `TestNotchGreetsOnce` and `TestNotchGreetingKeptUnderPointer`. Both attach a real audio output, and this VM has no ALSA device (`Unknown PCM default`), so the notch never greets. That is independent of the playing-page frames. `internal/lyrics`, `internal/netease`, `internal/player`, and `internal/speech` passed.

## Next, on the Windows machine

1. If the ~32 uncommitted files should be part of the comparison, commit or copy them onto a branch first. This PR does not contain them.
2. Follow [CHECKLIST.md](CHECKLIST.md): one song, the sizes in the table, awake and rest stills, one motion clip at 1180×780.
3. Drop notes in this file under a dated heading. Then a later change can adjust blur, cover, or motion against those pairs.

Do not commit real covers, session files, or the Windows credential for `dev.egoist.cloudmusic/netease-session`.
