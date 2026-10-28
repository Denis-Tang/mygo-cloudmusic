package app

import (
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/shader"

	"netease-music-mygo/internal/lyrics"
	"netease-music-mygo/internal/netease"
)

// The playing page is one whole, as an album poster of the song: the cover
// sharp where it stands, at the left of a wide view or across the top of a
// tall one, melting into the cover's own blur and then into the backdrop's
// colors; the song's name, its singer and the line it sings at over the
// picture; the lyrics a click asks for, reading where there is room; the
// queue and the comments, drawers of the same picture; and the controls in
// one slim bar at the foot, which hides once the listener rests. The page
// keeps the window as it is: "全屏封面" is the page's, not the display's.

// fcBarH is how tall the bar at the foot of the playing page is; what the
// words over it keep from.
const fcBarH = 100

// npIdle is how long the playing page's tools wait for an act of the
// listener before they soften away: resting over the window hides them,
// as much as leaving it does.
const npIdle = 2750 * time.Millisecond

// blendSource paints the cover over its own blur, and, past the picture,
// into the backdrop itself: the cover sharp in Split×Split at the view's
// left or top, fading in its last part into the blur of itself, which
// goes on out of the picture; and then into the backdrop's own colors,
// which the view keeps behind it.
const blendSource = `package main

var Split float
var FadeX float

func Fragment(dst vec4, pos vec2) vec4 {
	s := rectSize()
	// The cover, sharp, in the square of Split at this corner of the view:
	// its own proportions kept, its edges cropped to fill the square, so
	// that no picture is stretched to it.
	c0 := image0Size()
	k0 := max(Split/c0.x, Split/c0.y)
	o0 := (vec2(Split, Split) - c0*k0) * 0.5
	sharp := image0Sample((pos - o0) / k0)
	// Its blur, over the whole view, that it goes on past the picture
	// instead of reaching its edge and stretching its last column out of
	// it: the picture melts into a view of its own colors.
	c1 := image1Size()
	k1 := max(s.x/c1.x, s.y/c1.y)
	o1 := (s - c1*k1) * 0.5
	blur := image1Sample((pos - o1) / k1)
	bgc := image2Sample(pos * image2Size() / s)
	// The fade begins in the picture's last part, so it is over before
	// the picture ends, and no edge of it shows.
	t := smoothstep(0.70, 0.96, pos.x/Split)
	if FadeX < 0.5 {
		t = smoothstep(0.70, 0.96, pos.y/Split)
	}
	c := mix(sharp, blur, t)
	// Past the picture, into the backdrop, which is the view's own.
	l := Split * 1.02
	t2 := smoothstep(l, Split*1.6, pos.x)
	if FadeX < 0.5 {
		t2 = smoothstep(l, Split*1.6, pos.y)
	}
	return mix(c, bgc, t2)
}
`

var blendShader = sync.OnceValue(func() *shader.Shader { return shader.MustCompile(blendSource) })

// immersive fills the playing page: the cover as a poster of the song, the
// words over it, the lyrics and the drawers, and the bar at the foot. It
// is the same view in every pane; the panes only say what shows over the
// picture.
func (a *app) immersive(c *ui.Context, tr *netease.Track, art *artwork, accent ui.Color, w, h float32, pointerIn bool) {
	// The picture itself: the cover sharp, its blur, the backdrop, drawn
	// over the protections the moment the backdrop is asked for.
	ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough().Draw(func(p *ui.Painter, r ui.Rect) {
		S := min(r.W, r.H)
		if sharp := bitmap(a.art.get(netease.Sized(tr.Album.PicURL, 1024), false)); sharp != nil {
			blendShader().Draw(p, r, shader.Options{
				Images: [4]*ui.Bitmap{sharp, art.backdrop, safeCanvas(a.backdropCanvas())},
				Uniforms: map[string]any{
					"Split": S,
					"FadeX": b2f(r.W >= r.H),
				},
			})
		}
	})

	// The reading protections: the top, under the window's buttons; the
	// foot, broad and smooth, over the bar and the song's name, so that
	// white reads on the brightest of covers too.
	ui.Box(c).Absolute().Top(0).Left(0).Right(0).Height(h * 0.16).PassThrough().Draw(func(p *ui.Painter, r ui.Rect) {
		p.FillGradient(r, ui.LinearGradient{Angle: 180, Stops: []ui.GradientStop{
			{Color: ui.RGBA(0, 0, 0, 0.30), At: 0}, {Color: ui.RGBA(0, 0, 0, 0), At: 1},
		}}, 0)
	})
	ui.Box(c).Absolute().Left(0).Right(0).Bottom(0).Top(h * 0.44).PassThrough().Draw(func(p *ui.Painter, r ui.Rect) {
		p.FillGradient(r, ui.LinearGradient{Angle: 180, Stops: []ui.GradientStop{
			{Color: ui.RGBA(0, 0, 0, 0), At: 0.28}, {Color: ui.RGBA(0, 0, 0, 0.34), At: 0.62}, {Color: ui.RGBA(0, 0, 0, 0.48), At: 1},
		}}, 0)
	})

	// The picture's click shows and hides the lyrics, over the band of
	// picture above the words: under the words and the bar that come
	// after it, which take the clicks of themselves.
	b := ui.ButtonBase(c.Key("fc-cover")).Absolute().Top(topBar(c)).Left(0).Right(0).Bottom(h * 0.40).
		FocusRing(false).Label(coverOverlayLabel(a.npPane == paneLyrics))
	if b.Clicked() {
		if a.npPane == paneLyrics {
			a.npPane = paneCover
		} else {
			a.npPane = paneLyrics
		}
	}

	// The song's name, its singer, and the line it sings at, over the
	// picture above the bar. Reading the lyrics, that line reads there,
	// and the single line steps aside.
	if a.npPane != paneLyrics {
		a.infoColumn(c, tr, w, h)
	}

	// The lyrics, reading where there is room.
	if a.npPane == paneLyrics {
		a.reading(c, tr, w, h)
	} else if a.npPane != paneCover {
		// The queue, the comments, what a podcast is about or its
		// captions: a drawer of the same picture.
		a.drawer(c, tr, w, h)
	}

	// The bar at the foot, hiding once the listener rests.
	a.immBar(c, tr, accent, pointerIn)
}

// safeCanvas is the canvas's bitmap, or absent, as a shader reads it.
func safeCanvas(cn *ui.Canvas) *ui.Bitmap {
	if cn == nil {
		return nil
	}
	return cn.Bitmap()
}

// infoCol is how tall the information of the song is, name, singer and
// line, so the bar may keep its room from it.
func infoCol(compact bool) float32 {
	if compact {
		return 118
	}
	return 158
}

// infoColumn sets the song's name, its singer and the line it sings at,
// over the picture, above the bar: big to read, kept to a width, and
// nearer the foot of a small window than of a large one.
func (a *app) infoColumn(c *ui.Context, tr *netease.Track, w, h float32) {
	compact := h < 680
	name := float32(34)
	artist, line := float32(16), float32(20)
	if compact {
		name, artist, line = 26, 14, 18
	}
	width, lineWidth := min(w*0.64, 720), min(w*0.64, 640)
	ui.Column(c).Absolute().Left(0).Right(0).Bottom(fcBarH + 24).AlignItems(ui.Center).Gap(6).Height(infoCol(compact)).Children(func() {
		if r := a.pl.Radio(); r != "" {
			a.radioChip(c, r)
		}
		ui.Text(c, tr.Name).FontSize(name).FontWeight(750).LetterSpacing(-0.4).MaxLines(2).LineHeight(1.2).
			Width(width).TextAlign(ui.Center)
		a.artistLinks(c, tr, white.Alpha(0.85), white).FontSize(artist).SingleLine()
		a.nowLine(c, tr, line, lineWidth)
	})
}

// nowLine is the line of the lyrics the song sings at, under the song's
// name, or what an instrumental shows instead. It draws only on the
// cover's state, where the lyrics are not being read.
func (a *app) nowLine(c *ui.Context, tr *netease.Track, size, width float32) {
	if tr.ProgramID != 0 {
		return
	}
	pl := a.pl
	lines, _ := pl.Lyrics()
	if len(lines) == 0 {
		// No lyrics of this song at NetEase: not theirs to sing.
		ui.Text(c, "纯音乐，请欣赏").FontSize(size * 0.7).TextColor(white.Alpha(0.62)).SingleLine()
		return
	}
	pos := pl.Position()
	cur := lyrics.Index(lines, pos)
	line := lines[cur]
	ui.Column(c).Gap(2).Width(width).AlignItems(ui.Center).Children(func() {
		ui.Text(c, line.Text).FontSize(size).FontWeight(650).TextColor(white.Alpha(0.94)).MaxLines(2).LineHeight(1.3).TextAlign(ui.Center)
		if line.Translation != "" {
			ui.Text(c, line.Translation).FontSize(size * 0.62).TextColor(white.Alpha(0.66)).MaxLines(1).TextAlign(ui.Center)
		}
	})
	// Again as the next line begins, and a little on, that the redraw of
	// a line that lights word by word keeps smooth.
	if pl.Playing() && cur+1 < len(lines) {
		c.After(lines[cur+1].At - pos + 20*time.Millisecond)
	}
	if pl.Playing() && len(line.Words) > 0 {
		c.After(40 * time.Millisecond)
	}
}

// coverOverlayLabel says what the picture's click does now.
func coverOverlayLabel(lyricsShown bool) string {
	if lyricsShown {
		return "隐藏歌词，回到封面"
	}
	return "显示歌词"
}

// reading shows the song's lyrics where there is room: at the right of a
// wide view, over a soft backing where there is not. The read keeps from
// the top of the window and the bar at the foot; the lines follow the
// song as it sings, and a click on one seeks to it.
func (a *app) reading(c *ui.Context, tr *netease.Track, w, h float32) {
	wide := w >= 1120 && w/h >= 1.3
	panel := ui.Box(c.Key("reading"))
	if wide {
		panel.Absolute().Top(topBar(c) + 6).Bottom(fcBarH + 12).Right(30).Grow(0).Width(min(max(w*0.3, 420), 620))
	} else {
		panel.Absolute().Top(topBar(c) + 6).Bottom(fcBarH + 12).Left(max(24, (w-640)/2)).Right(max(24, (w-640)/2))
		panel.Radius(18).Background(ui.RGBA(0, 0, 0, 0.34)).Border(1, white.Alpha(0.08)).
			Shadow(0, 24, 60, -20, ui.RGBA(0, 0, 0, 0.5))
	}
	panel.AlignItems(ui.Stretch)
	panel.Children(func() { a.lyricsPane(c, tr, h) })
}

// drawer shows what aids the playing page: the queue, the comments, what
// a podcast is about, or its captions. One at a time, they cover the
// right of the view where there is room, and most of it where there is
// not, the bar at the foot kept beneath.
func (a *app) drawer(c *ui.Context, tr *netease.Track, w, h float32) {
	wide := w >= 1000
	panel := ui.Column(c.Key("drawer"))
	if wide {
		panel.Absolute().Top(topBar(c) + 6).Bottom(fcBarH + 12).Right(24).Grow(0).Width(min(420, max(360, w*0.3)))
	} else {
		panel.Absolute().Top(topBar(c) + 6).Bottom(fcBarH + 12).Left(24).Right(24)
	}
	title := "待播清单"
	if a.npPane == paneComments {
		title = a.commentsTab()
	} else if a.npPane == paneProgram {
		title = "节目简介"
	} else if a.npPane == paneCaptions {
		title = "字幕"
	}
	panel.Radius(18).Background(ui.RGBA(14, 14, 16, 0.82)).Border(1, white.Alpha(0.1)).
		Shadow(0, 30, 70, -20, ui.RGBA(0, 0, 0, 0.5)).
		Padding(10, 14, 12, 14).AlignItems(ui.Stretch).TextColor(white)
	panel.Children(func() {
		// The drawer's head: what it is, and the closing of it, which the
		// same of its entry, and the escape key, do also.
		ui.Row(c).Height(34).Padding(0, 4).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, title).FontSize(15).FontWeight(700).Grow(1).SingleLine()
			if iconButton(c.Key("close-drawer"), iconX, 16, white.Alpha(0.6), "收起").Clicked() {
				a.npPane = paneCover
			}
		})
		// Its body, which keeps its scroll of itself.
		ui.Box(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
			switch a.npPane {
			case paneQueue:
				a.queuePane(c)
			case paneComments:
				a.commentsPane(c, tr)
			case paneProgram:
				a.programPane(c, tr)
			default:
				a.captionsPane(c, tr, h)
			}
		})
	})
}

// immBar is the bar at the foot of the playing page: the progress, with
// the times it stands between, and the controls, the summary of what
// plays at their left, the entries of the page, the quality, its volume
// and the rest at their right. It shows while the listener is at it, and
// softens away once they rest.
func (a *app) immBar(c *ui.Context, tr *netease.Track, accent ui.Color, pointerIn bool) {
	pl := a.pl
	awake := a.npAwake(c)
	if awake {
		// Asleep again after the wait, a build turns the tools to their
		// hiding pose.
		c.After(npIdle - c.Now().Sub(a.npActiveAt) + 10*time.Millisecond)
	}
	reveal(ui.Column(c.Key("fc-bar")).Absolute().Left(0).Right(0).Bottom(18).Padding(0, 28).Gap(6), awake).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, clock(pl.Position())).FontSize(12).FontFeatures("tnum").TextColor(white.Alpha(0.55)).Width(40).TextAlign(ui.End)
			a.scrubber(c.Key("fc-scrub"), &a.coverScrub, white.Alpha(0.24), white.Alpha(0.92), 12, accent).Grow(1)
			ui.Text(c, clock(pl.Duration())).FontSize(12).FontFeatures("tnum").TextColor(white.Alpha(0.55)).Width(40)
		})
		ui.Row(c).Height(52).AlignItems(ui.Center).Gap(14).Children(func() {
			// The summary of what plays, at the left, and what one may
			// think of it; the words it takes have room, and give it up.
			ui.Row(c).Grow(1).Basis(0).MinWidth(120).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(44, 44).Radius(8).Children(func() { a.coverFill(c, tr.Album.PicURL, 120, 8) })
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
					ui.Text(c, tr.Name).FontSize(13.5).FontWeight(700).SingleLine()
					a.artistLinks(c, tr, white.Alpha(0.6), white).FontSize(11.5).SingleLine()
				})
				a.likeButton(c.Key("like"), tr, 17, white.Alpha(0.7))
			})
			// The playing and the going, in the middle, where they stay
			// whatever the words beside them do.
			a.transport(c, white, white.Alpha(0.45), white, 24)
			// The playing page's entries, once each; the quality, the
			// volume and the rest at the right.
			ui.Row(c).Grow(1).Basis(0).MinWidth(120).Justify(ui.End).AlignItems(ui.Center).Gap(6).Children(func() {
				a.paneButton(c, paneLyrics, iconLyrics, "歌词", true)
				a.paneButton(c, paneQueue, iconQueue, "待播清单", true)
				if a.countButton(c, paneComments).Clicked() {
					a.choosePane(paneComments)
				}
				// An episode has its description and its captions for the song's
				// lyrics and queue.
				if tr.ProgramID != 0 {
					a.paneButton(c, paneProgram, iconInfo, "简介", true)
					if a.captionsShown() {
						a.paneButton(c, paneCaptions, iconCaptions, "字幕", true)
					}
				}
				qualityBadge(c, pl.Stream())
				a.volumeButton(c.Key("volume"), white.Alpha(0.7))
				a.songMenu(c.Key("more"), tr, 18, white.Alpha(0.7))
			})
		})
	})
}

// npAwake reports whether the listener acted on the playing page lately,
// whose tools show while it is awake: moving, clicking or scrolling wakes
// it, and so does dragging the progress, or the volume's popover.
func (a *app) npAwake(c *ui.Context) bool {
	// Reading, or a drawer's open: the bar keeps still at the foot.
	if a.npPane != paneCover || a.npScrub.active || a.coverScrub.active || a.volumeOpen {
		return true
	}
	return c.Now().Sub(a.npActiveAt) < npIdle
}

// backdropCanvas is the canvas the backdrop paints into, or nil before
// the first song painted it.
func (a *app) backdropCanvas() *ui.Canvas { return a.backdropCache.canvas }
