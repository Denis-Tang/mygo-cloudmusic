# Playing-page parity

## Checkout

Branch `fix/player-ui-parity`, rebased onto `origin/main` at `5b786a7` (`Refine page typography, player bar and search`). That tip also includes `1df9916` (`Improve playback buffering, collections and shared layout`).

The first look at this cloud checkout, then at `106cd3c`, found a clean tree. No local WIP on remote at that time; the user’s uncommitted files were only on their PC until they pushed. Those two commits are the pushed work this branch now contains. Anything still uncommitted on the PC is still absent.

`本地开发说明-改代码看这里.md` on this tip still names `internal/app/ui_stage.go`, `ui_stage_styles.go`, and settings `LyricsStage`, `StageStyle`, `StageSparks`, `Fog`, and `Smoke`. Those files and fields are not in the Go tree at `5b786a7`. `ui_nowplaying.go` is 581 lines. A comment on `app.npPane` still says `paneStage`, and that constant is gone.

## What the playing page does now

The page that opens with Ctrl+L is `nowPlayingView` (`internal/app/ui_nowplaying.go`) and `immersive` (`internal/app/ui_immersive.go`).

- The cover is scaled to cover the whole window (proportions kept, edges cropped). It is not a centered square card.
- Wider than height × 1.15, the same picture stays sharp through the left third and falls out of focus from 34% to 80% of the width (`smoothstep` in `blendSource`). Square and tall windows stay sharp across. The crop is still centered cover-fit: proportions kept, overflow cut equally.
- The soft side is `stageBlur` (`internal/app/artwork.go`): the real cover shrunk to 256×256, three box blurs of radius 14, its own colors. The old 48×48 saturated `blurImage` is only the faint wash under the page, and the shader does not use it. Until `stage` has loaded, the sharp cover stands in for the soft side.
- Under that, `backdrop` (`internal/app/backdrop.go`) paints a deep gradient of the cover’s main colour, lighter at the top, plus the blurred cover at 10% opacity, into a canvas an eighth of the window across. The full-bleed cover is drawn over it, so the gradient shows only where the cover does not. Comments in `backdrop.go` describe this gradient as matching NetEase’s player; the picture on top is a later full-bleed poster.
- Title, artist, and the current lyric line are centered just above the bottom bar (name 34px, or 26px when the window is shorter than 680). Lyrics sit in a right column when the window is at least 1120px wide and at least 1.3 times as wide as it is tall; otherwise they are a centered panel.
- The playing-page bar is now a full-width strip flush with the bottom, 84px tall (`playerBarHeight`), filled with `rgba(20, 20, 22, 0.6)`. It has no cover thumbnail. The song name is 17px and the artist 14px. The progress line is a full-width scrubber along the top edge of that strip (`Top(-8)`). Elapsed and remaining time show only while the pointer is on the scrubber. Transport, like, collect, quality, volume, and “更多” sit in the strip. The back control is a 40px round icon, still labeled “返回”, with no word beside the chevron. The strip hides after 2.75s idle.
- Opening and closing spring the whole layer from 60px below (`npSpring`, 450ms, bounce 0.08).

`5b786a7` changed type sizes and this bar. The cover fill on top of that is now the defocus above. The 84px bar is unchanged.

`npCover` in `internal/app/ui_cover.go` is the square card that tilts and flings. The playing page does not build it. Putting that card on the page would replace the full-window cover. The picture on screen is the real album bitmap in `immersive`.

The bar on ordinary pages (`playerBar` in `internal/app/ui_player.go`) is a separate 84px strip. Its progress line is also full width, along the top of the strip, and the strip is a solid page background with a hairline instead of the old frosted scroll edge. It is not built while the playing page has settled, so the frames below do not show it.

Likely places a paired screenshot will disagree, to confirm rather than to fix yet:

1. Official art is often described as one picture, sharper on one side. Here that side is the left, and the right is the same centered crop out of focus. A square card over a fully blurred field is the unused `npCover` path.
2. The falloff (sharp until 34% of the width, fully soft by 80%) and the box blur are stand-ins. See the questions below.
3. Motion: no disc spin and no pointer tilt on the page that actually shows.
4. Text sits low and centered. Lyric layout still changes with the thresholds above. The bar was not part of this pass.

## Screenshots from this VM

The official NetEase Windows client does not run here. This app was not logged in and was not given a live song.

`ui.NewTester` drew the playing page without a window. `TestNowPlayingAspectFrames` was run again after the cover change, with `NM_FRAMES` set to `docs/ui-parity/frames/mygo`. The cover is synthetic: a 512×512 wash, a disc, and a red square at the cover’s top left. The PNGs are this app only. The bar is still the full-width strip from `5b786a7`.

What those frames show, which matches the code above:

- `16x9-1280x720.png` and `16x9-1920x1080.png`: the disc’s left edge stays sharp and its right edge softens into the same picture, still a disc. The red square is cropped off the top.
- `4x3-1024x768.png` and `default-1180x780.png`: same side blur. The red square is cut by the top edge.
- `1x1-800x800.png` and `1x1-1080x1080.png`: the red square is fully in frame, and the disc stays sharp all the way around. No side blur.
- `9x16-720x1280.png`: the red square is cropped off the side. The disc stays sharp.
- `lyrics-16x9-1280x720.png`: the lyric line sits on the right, over the blur.
- `lyrics-1x1-800x800.png`: the lyric line sits in a centered dark panel.
- The bottom strip is full width, flush with the window, and has no cover thumbnail. The pointer in these frames sits in the middle of the picture, so the time tooltip on the progress line is not showing. Controls are the awake state. Nothing here is a recording of the open or close spring.

Left-edge stair-steps, where the disc is sharp, come from drawing a 512×512 fixture into a larger window. A real 1536 cover would be smoother there. The soft right side is the 256×256 defocus.

## Tests on this VM

`TestWideCoverSoftensTowardTheRight` passed five times: on a wide window the left stays the sharp cover and a point in the falloff picks up the other half; a square window does not. `TestFullCoverShowsWhileHighResolutionLoads` still passes. `TestNowPlayingAspectFrames` was run with `NM_FRAMES` set to `docs/ui-parity/frames/mygo`. `go test ./internal/app/ -skip TestNotch` passed.

## Open questions

These are the official details this pass did not guess past. The numbers above are the stand-in.

1. On a wide window, should the cover stay center-cropped (what this page does), or should the left edge of the art stay pinned to the left of the window?
2. How far across should the picture stay fully sharp? This page is sharp until 34% of the width and fully soft by 80%.
3. How strong is the soft side: a light defocus that still reads as the photo, or a heavy blur? This page is three box passes of radius 14 on a 256×256 copy.

## Next, on the Windows machine

1. Follow [CHECKLIST.md](CHECKLIST.md): one song, the sizes in the table, awake and rest stills, one motion clip at 1180×780. Put the pointer on the progress line for one still so the time tooltip is in the shot.
2. If more uncommitted files remain on the PC, they are not in `5b786a7`.
3. Drop notes in this file under a dated heading. Then a later change can adjust blur, cover, or motion against those pairs.

Do not commit real covers, session files, or the Windows credential for `dev.egoist.cloudmusic/netease-session`.
