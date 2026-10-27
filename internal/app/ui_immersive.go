package app

import (
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/shader"

	"netease-music-mygo/internal/lyrics"
	"netease-music-mygo/internal/netease"
)

// The immersive page of Now Playing: the album cover the whole view, as
// NetEase's "全屏封面" shows it — the cover sharp at its left, full height,
// easing as it goes right into the cover blurred, which fills the rest;
// the song's name, its singer and the line it sings at, set over the
// picture; the controls in a slim bar at the foot, which the pointer
// brings. The picture's click shows the song's lyrics over it, and click
// again takes them back.

// blendSource paints the cover over its own blur: the cover sharp where it
// lies, easing into the blurred copy of itself beyond Split, its right
// edge, over Feather, in DIPs.
const blendSource = `package main

var Split float
var Feather float

func Fragment(dst vec4, pos vec2) vec4 {
	s := rectSize()
	// The cover is a square of the view's height, at the left: its
	// pixels, a uniform scale of image0's, by the height's pixels.
	sharp := image0Sample(pos * image0Size() / vec2(s.y, s.y))
	// The blur, drawn over the whole view: where the cover goes as it
	// fades.
	blur := image1Sample(pos * image1Size() / s)
	t := clamp((pos.x-Split)/max(Feather, 0.001), 0.0, 1.0)
	return mix(sharp, blur, t)
}
`

var blendShader = sync.OnceValue(func() *shader.Shader { return shader.MustCompile(blendSource) })

// immersive fills this pane of Now Playing with the cover and the song.
func (a *app) immersive(c *ui.Context, tr *netease.Track, art *artwork, accent ui.Color, w, h float32, pointerIn bool) {
	// The cover itself: sharp at the left, its own blur beyond.
	ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough().Draw(func(p *ui.Painter, r ui.Rect) {
		sharp := bitmap(art)
		if sharp == nil {
			return
		}
		// The cover, a square of the height, at the left; a narrower
		// window crops it at the right, as the official one does.
		split := min(r.H, r.W)
		feather := min(r.W*0.35, r.H*0.6)
		blendShader().Draw(p, r, shader.Options{
			Images:   [4]*ui.Bitmap{sharp, art.backdrop},
			Uniforms: map[string]any{"Split": split, "Feather": feather},
		})
	})

	// The picture's click shows and hides the lyrics: under the words and
	// the bar that come after it, which take the clicks of themselves.
	// The click layer lies in the band of the cover above the words and
	// the bar: over nothing that takes a click of its own.
	b := ui.ButtonBase(c.Key("fc-cover")).Absolute().Top(topBar(c)).Left(0).Right(0).Bottom(h * 0.42).
		FocusRing(false).Label(coverOverlayLabel(a.ovLyrics))
	if b.Clicked() {
		a.ovLyrics = !a.ovLyrics
	}

	// The lyrics over the picture, once a click asked for them.
	if a.ovLyrics {
		a.ovLines(c, tr, w, h)
	}

	// The song's name, its singer, and the line it sings at, over the
	// picture, above the foot where the bar goes. On a radio, the chip
	// that leaves it shows over the name.
	ui.Column(c).Absolute().Left(0).Right(0).Bottom(150).AlignItems(ui.Center).Gap(4).Children(func() {
		if r := a.pl.Radio(); r != "" {
			a.radioChip(c, r)
		}
		ui.Text(c, tr.Name).FontSize(22).FontWeight(750).SingleLine()
		a.artistLinks(c, tr, white.Alpha(0.8), white).FontSize(14).SingleLine()
		a.nowLine(c, tr)
	})

	// The controls, showing while the pointer is in the window.
	reveal(ui.Column(c.Key("fc-bar")).Absolute().Left(0).Right(0).Bottom(22).Padding(0, 36).Gap(10), pointerIn).Children(func() {
		a.scrubber(c.Key("fc-scrub"), &a.coverScrub, white.Alpha(0.24), white.Alpha(0.9), 10, accent.Mix(white, 0.35))
		ui.Row(c).AlignItems(ui.Center).Gap(18).Children(func() {
			// The song, and what one may think of it, at the left.
			ui.Row(c).Grow(1).Basis(0).MinWidth(0).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Children(func() {
					ui.Text(c, tr.Name).FontSize(13.5).FontWeight(700).SingleLine()
					a.artistLinks(c, tr, white.Alpha(0.6), white).FontSize(11.5).SingleLine()
				})
				a.likeButton(c.Key("like"), tr, 17, white.Alpha(0.7))
				if a.countButton(c, paneComments).Clicked() {
					a.choosePane(paneComments)
				}
			})
			a.transport(c, white, white.Alpha(0.45), white, 22)
			if iconButton(c.Key("queue"), iconQueue, 15, white.Alpha(0.7), "待播清单").Clicked() {
				a.choosePane(paneQueue)
			}
			// The quality, the volume and the rest, at the right.
			ui.Row(c).Grow(1).Basis(0).MinWidth(0).Justify(ui.End).AlignItems(ui.Center).Gap(12).Children(func() {
				qualityBadge(c, a.pl.Stream())
				a.volumeButton(c.Key("volume"), white.Alpha(0.7))
				a.songMenu(c.Key("more"), tr, 18, white.Alpha(0.7))
			})
		})
	})
}

// coverOverlayLabel says what the picture's click does now.
func coverOverlayLabel(lyrics bool) string {
	if lyrics {
		return "隐藏歌词"
	}
	return "显示歌词"
}

// nowLine is the line of the lyrics the song sings at, under the song's
// name, or what an instrumental shows instead.
func (a *app) nowLine(c *ui.Context, tr *netease.Track) {
	if tr.ProgramID != 0 {
		return
	}
	lines, _ := a.pl.Lyrics()
	if len(lines) == 0 {
		ui.Text(c, "纯音乐，请欣赏").FontSize(14).TextColor(white.Alpha(0.55))
		return
	}
	pl := a.pl
	pos := pl.Position()
	cur := lyrics.Index(lines, pos)
	line := lines[cur]
	ui.Column(c).AlignItems(ui.Center).Gap(2).Children(func() {
		ui.Text(c, line.Text).FontSize(14).FontWeight(600).TextColor(white.Alpha(0.9)).SingleLine()
		if line.Translation != "" {
			ui.Text(c, line.Translation).FontSize(12).TextColor(white.Alpha(0.62)).SingleLine()
		}
	})
	// Again as the next line begins, and a little on, that the redrawing
	// of the line that lights word by word keeps smooth.
	if pl.Playing() && cur+1 < len(lines) {
		c.After(lines[cur+1].At - pos + 20*time.Millisecond)
	}
	if pl.Playing() && len(line.Words) > 0 {
		c.After(40 * time.Millisecond)
	}
}

// ovLines is the song's lyrics, over the picture, as the picture's click
// asked for them: the line sung lit, and brought to a third of the way
// down once it changes; a click on one seeks to it. Its scroll, glide and
// where the line got to are the main window's own, for the picture's
// lyrics only, which the lyrics pane of the player bar keeps to itself.
func (a *app) ovLines(c *ui.Context, tr *netease.Track, w, h float32) {
	pl := a.pl
	lines, _ := pl.Lyrics()
	if len(lines) == 0 {
		return
	}
	pos := pl.Position()
	cur := lyrics.Index(lines, pos)
	if pl.Playing() && cur+1 < len(lines) {
		c.After(lines[cur+1].At - pos + 20*time.Millisecond)
	}
	if a.ovTrack != tr.ID {
		a.ovTrack, a.ovAt = tr.ID, -2
		a.ovScroll, a.ovGlide = ui.ScrollState{}, lyricGlide{}
	}
	lag := a.ovGlide.step(c, &a.ovScroll)
	scroll := ui.Scroll(c.Key("ov-lyrics")).TrackScroll(&a.ovScroll).
		Absolute().Left((w-min(w*0.6, 640))/2).Width(min(w*0.6, 640)).
		Top(h*0.12).Padding(0, 8, h*0.5, 0)
	a.ovText(c, scroll, lines, pos, cur, lag)
}

// ovText fills scroll with the lyrics that follow what plays: the same as
// timedLines, but keeping to the picture's own scroll and glide, where
// timedLines keeps to the lyrics pane's.
func (a *app) ovText(c *ui.Context, scroll ui.Element, lines []lyrics.Line, pos time.Duration, cur int, lag float32) {
	pl := a.pl
	var active ui.Element
	scroll.Children(func() {
		ui.Column(c).Gap(8).Translate(0, -lag).Children(func() {
			for i, l := range lines {
				line := ui.ButtonBase(c.Key(i)).Column().AlignItems(ui.Center).Padding(10, 12).Radius(12).Gap(6).FocusRing(false).Label(l.Text)
				alpha := float32(0.32)
				if i == cur {
					alpha = 1
				} else if line.Hovered() {
					alpha = 0.6
				}
				text := ui.Column(c).AlignItems(ui.Center)
				text.AnimatePose(ui.Pose{Fade: 1 - alpha}, ui.Animation{Duration: 380 * time.Millisecond})
				line.Children(func() {
					text.Children(func() {
						if i == cur && len(l.Words) > 0 {
							ui.RichText(c).FontSize(30).FontWeight(800).LineHeight(1.3).LetterSpacing(-0.4).Children(func() {
								for wi, wd := range l.Words {
									lit := clamp01(float32((pos - wd.Start).Seconds() / max(0.05, (wd.End-wd.Start).Seconds())))
									ui.Text(c.Key(wi), wd.Text).TextColor(white.Alpha(0.38 + 0.62*lit))
								}
							})
							if pl.Playing() {
								c.After(40 * time.Millisecond)
							}
						} else {
							ui.Text(c, l.Text).FontSize(30).FontWeight(800).LineHeight(1.3).LetterSpacing(-0.4)
						}
						if l.Translation != "" {
							ui.Text(c, l.Translation).FontSize(17).FontWeight(600).TextColor(white.Alpha(0.7))
						}
					})
				})
				if line.Clicked() {
					pl.Seek(l.At)
				}
				if i == cur {
					active = line
				}
			}
		})
	})
	// Bring the line sung to a third of the way down, once it changed.
	if cur != a.ovAt {
		box := scroll.Bounds()
		target := float32(0)
		if active.Valid() {
			if lb := active.LayoutBounds(); lb.H > 0 && box.H > 0 {
				target = lb.Y - box.Y + a.ovScroll.Y - box.H*0.3
			} else {
				return // not laid out yet: the next frame scrolls
			}
		}
		a.ovAt = cur
		a.ovGlide.to(c, &a.ovScroll, min(max(0, target), a.ovScroll.MaxY))
	}
}
