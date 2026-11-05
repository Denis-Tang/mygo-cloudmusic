# Playing-page comparison

Compare the official NetEase Cloud Music Windows client with this app’s playing page (`正在播放`). Use one song, paused on the same lyric line, then the window shapes below. This file is the capture list. Progress is in [STATUS.md](STATUS.md).

The cloud checkout cannot run the official Windows client. Frames under `frames/mygo/` are this app only, drawn by `ui.NewTester` from a synthetic cover. They are not a substitute for the paired captures below.

## Same song

Pick a song that both clients can play. Write it down before capturing:

- title, artist, album
- NetEase song id (the number in the official share link, or the id this app logs)
- paused position, as `m:ss` and as the lyric line on screen
- account quality (standard / higher / lossless), only so the two clients are not obvious about a missing stream

Prefer a colourful cover that is not a flat field, and a song with a translation line, so placement of the second line is visible.

Do not commit those captures if they contain the real cover. Real album art stays on the machine that took it. The synthetic frames in this folder are safe to keep in git.

## What each capture is

Take two stills per shape:

1. **awake** — pointer just moved, controls visible. In this app the bar and the back pill hide after 2.75s (`npIdle` in `internal/app/ui_immersive.go`).
2. **rest** — pointer still for at least 3 seconds, so this app has hidden the bar.

Then one short recording (about 5 seconds) for motion, at the default size only:

- open the playing page
- change to the next song
- let one lyric line advance

Name files `{shape}-{awake|rest}.png` and `{shape}-motion.mp4`. Put official files in `captures/official/` and this app’s files in `captures/mygo/`. Create those directories locally; they are not in git.

## Window shapes

Sizes are the window content box in CSS pixels. This app’s window has a hidden title bar (`TitleBarHidden` in `internal/app/app.go`), so the content box is the whole view. Match that box on the official client as closely as the window manager allows, and write the real size in the filename if it differs. Set Windows display scale to 100% for the session if you can; otherwise record the scale next to the shot.

| id | content px | ratio | why this shape |
| --- | --- | --- | --- |
| `default` | 1180×780 | about 3:2 | this app’s default window |
| `min` | 880×580 | about 3:2 | this app’s minimum window |
| `16x9-md` | 1280×720 | 16:9 | common laptop size |
| `16x9-lg` | 1920×1080 | 16:9 | large window, same ratio |
| `4x3` | 1024×768 | 4:3 | just wide enough for this app’s side blur |
| `1x1-md` | 800×800 | 1:1 | square; this app does not fade the cover into blur |
| `1x1-lg` | 1080×1080 | 1:1 | same ratio, larger type and cover crop |
| `9x16` | 720×1280 | 9:16 | tall; cover crop is top and bottom |
| `lyrics-16x9` | 1280×720 | 16:9 | lyrics open |
| `lyrics-1x1` | 800×800 | 1:1 | lyrics open on a square |

On this app, Ctrl+L toggles `正在播放` (menu 控制 → 正在播放). The playing page opens only while a song is current. Click the picture to toggle lyrics. On the official client, open the same song’s full playing page, not the mini bar and not a separate fullscreen monitor mode, unless you label that shot `fullscreen`.

## What to look at

Check each pair, official on the left. Write a one-line note in STATUS rather than restyling from memory.

**Cover**

- Square card with margins, or the picture cropped until it fills the window?
- On a wide window, is the whole picture sharp, or does one side soften?
- On a square and on a tall window, which edges of the cover are cut off? The red corner mark in the synthetic frames sits at the cover’s top left; a real cover needs its own landmark (a face, a title on the art).
- Is there a disc, a spin, or a tilt toward the pointer? This app’s tilt, glare, and fling live in `npCover` (`internal/app/ui_cover.go`) and nothing calls it. The page on screen is `immersive` (`internal/app/ui_immersive.go`).

**Blur**

- Does blur fill the window behind a sharp card, or is the sharp picture itself the background?
- How heavy is the blur, and does it band or look like a small image scaled up?
- Is there a separate colour wash (a gradient of one hue) on top of the blur?
- This app paints a gradient backdrop in `backdrop.go`, then draws the cover over the whole window. On a wide window the shader keeps the cover sharp on the left and mixes in a 48×48 blurred copy from 42% to 72% of the width (`blendSource` in `ui_immersive.go`). The mix runs only when width > height × 1.15. Square and tall windows stay sharp. The blur is three box passes of radius 5 on a 48×48 shrink, with saturation pushed (`blurImage` in `artwork.go`).

**Type and controls**

- Where the title, artist, and current lyric line sit (under a card, centered low, or to one side).
- Whether the translation line is present, and its size.
- Progress bar, times, transport, small cover thumbnail, quality, volume.
- Lyrics: a column on the right, or a panel over the middle. Here, the right column is used when width ≥ 1120 and width/height ≥ 1.3 (`reading` in `ui_immersive.go`). Otherwise the lyrics are a centered panel.

**Motion** (from the recording, not from the stills)

- How the page enters and leaves. Here the layer springs from 60px below with a fade (`npSpring`, 450ms, bounce 0.08, in `ui_nowplaying.go`).
- How the next cover arrives (slide, fade, or a new full-bleed picture).
- How the current lyric line moves.

## This app, headless, synthetic cover

From the repo root, with Go 1.27.3 or newer:

```bash
mkdir -p docs/ui-parity/frames/mygo
NM_FRAMES="$PWD/docs/ui-parity/frames/mygo" go test ./internal/app/ -run TestNowPlayingAspectFrames -count=1
```

`go test` runs with the package directory as its working directory. A relative `NM_FRAMES` would write under `internal/app/`. Use the absolute path above.

`TestNowPlayingAspectFrames` skips unless `NM_FRAMES` is set. It does not call NetEase. The cover is drawn in the test (512×512). The window shapes match the table above, except the filenames use the pixel size (`16x9-1280x720.png`, and so on). Controls are awake. Two extra files open the lyrics pane. The bar’s small cover is the same fixture. Any other size stays empty instead of being fetched.

These frames show layout, crop, and the wide-window blur. They do not show a real song, the official client, or motion.

## Official client and this app, on Windows

The official client and a logged-in window of this app have to be captured on Windows. From the repo directory (the copy that has the uncommitted work, if that is the tree you mean to compare):

```bash
go tool mygo dev
```

Play the chosen song, press Ctrl+L, resize to each content size, and save the stills described above. Pause at the same lyric line in both clients before each still. For the motion clip, start both from the same line and press play together as well as you can; exact sync is not required if the lyric line is readable.

`go tool mygo dev` keeps the existing login as long as `mygo.json` still uses the app id `dev.egoist.cloudmusic`. Session files live under `%APPDATA%\MyGo 云音乐\`. Do not copy those files into this repo.

To refresh only this app’s synthetic frames on Windows, use the `NM_FRAMES` command above. It does not need a song or a login.
