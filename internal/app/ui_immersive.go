package app

import (
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/shader"

	"netease-music-mygo/internal/lyrics"
	"netease-music-mygo/internal/netease"
)

// The playing page is one whole, as the album's own poster: the cover
// filling it, its proportions kept and its edges cropped to it, sharp at
// the left of a wide view and easing into its own blur at the right, whole
// where the window is not wide; the song's name, its singer and the line it
// sings at over the picture; the lyrics a click asks for, reading where
// there is room; the queue and the comments, drawers of the same picture;
// and the controls in one slim bar at the foot, which hides once the
// listener rests. The page keeps the window as it is: "全屏封面" is the
// page's, not the display's.

// fcBarH is how tall the bar at the foot of the playing page is; what the
// words over it keep from.
const fcBarH = 100

// npIdle is how long the playing page's tools wait for an act of the
// listener before they soften away: resting over the window hides them,
// as much as leaving it does.
const npIdle = 2750 * time.Millisecond

// blendSource paints the cover over its own blur: the same cover, at the
// same size, sharp at the left of a wide view and easing into the blur
// that its right shows, so that the two are one picture rather than two
// rectangles, an edge of one set against the other.
const blendSource = `package main

func Fragment(dst vec4, pos vec2) vec4 {
	s := rectSize()
	// The cover, the whole view: its own proportions kept, its edges
	// cropped to fill it.
	c0 := image0Size()
	k0 := max(s.x/c0.x, s.y/c0.y)
	o0 := (s - c0*k0) * 0.5
	sharp := image0Sample((pos - o0) / k0)
	// The blur of the same cover, mapped just so, that the fade between
	// them crosses nothing but a softening of one picture.
	c1 := image1Size()
	k1 := max(s.x/c1.x, s.y/c1.y)
	o1 := (s - c1*k1) * 0.5
	blur := image1Sample((pos - o1) / k1)
	// A wide view keeps the picture whole at its left and eases it into
	// its blur before its right; a taller one keeps it whole.
	t := 0.0
	if s.x > s.y*1.15 {
		t = smoothstep(0.42, 0.72, pos.x/s.x)
	}
	return mix(sharp, blur, t)
}
`

var blendShader = sync.OnceValue(func() *shader.Shader { return shader.MustCompile(blendSource) })

// immersive fills the playing page: the cover as the poster of the song, the
// words over it, the lyrics and the drawers, and the bar at the foot. It
// is the same view in every pane; the panes only say what shows over the
// picture.
func (a *app) immersive(c *ui.Context, tr *netease.Track, art *artwork, accent ui.Color, w, h float32) {
	// The picture itself, over the backdrop the view keeps behind it: the
	// cover sharp and its own blur, the two of one size, that the left of
	// a wide view is the picture and its right the softness of the same
	// one. Until the blur has loaded, the sharp stands for it.
	ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough().Draw(func(p *ui.Painter, r ui.Rect) {
		// The picture is the whole view, so it is asked for at a size that
		// fills one: the service's own original where it has one.
		sharp := bitmap(a.art.get(netease.Sized(tr.Album.PicURL, 1536), false))
		if sharp == nil {
			return
		}
		soft := sharp
		if art != nil && art.backdrop != nil {
			soft = art.backdrop
		}
		blendShader().Draw(p, r, shader.Options{Images: [4]*ui.Bitmap{sharp, soft}})
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
	// after it, which take the clicks of themselves. Resting, it hides the
	// pointer with the controls.
	b := ui.ButtonBase(c.Key("fc-cover")).Absolute().Top(topBar(c)).Left(0).Right(0).Bottom(h * 0.40).
		FocusRing(false).Label(coverOverlayLabel(a.npPane == paneLyrics))
	if !a.npAwake(c) {
		b.Cursor(ui.CursorNone)
	}
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
	a.immBar(c, tr, accent, w)
}

// infoCol is how tall the information of the song is, name, singer and
// line, so the bar may keep its room from it: its room above the bar, the
// words a little nearer the foot of the picture than they once sat.
func infoCol(compact bool) float32 {
	if compact {
		return 78
	}
	return 96
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
	lines, loaded := pl.Lyrics()
	if len(lines) == 0 {
		// Still loading, it says nothing; none at NetEase, 纯音乐.
		if !loaded {
			return
		}
		ui.Text(c, "纯音乐，请欣赏").FontSize(size * 0.7).TextColor(white.Alpha(0.62)).SingleLine()
		return
	}
	pos := pl.Position()
	cur := lyrics.Index(lines, pos)
	if cur < 0 {
		// Before the song reaches its first line there is none to show; a
		// build once it does shows it. Indexing the lines as it was here
		// panicked on a song at its start.
		if pl.Playing() {
			c.After(lines[0].At - pos + 20*time.Millisecond)
		}
		return
	}
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
		panel.Radius(radiusPanel).Background(ui.RGBA(0, 0, 0, 0.34)).Border(1, white.Alpha(0.08)).
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
	panel.Radius(radiusPanel).Background(ui.RGBA(14, 14, 16, 0.82)).Border(1, white.Alpha(0.1)).
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

// immBarWide is how wide the playing page is for the bar to name its
// entries: narrower, their names would cost the volume and the rest their
// room, and they show as their icons alone.
const immBarWide = 1000

// immBar is the bar at the foot of the playing page: the progress, with
// the times it stands between, and the controls, the summary of what
// plays at their left, the entries of the page, the quality, its volume
// and the rest at their right. It shows while the listener is at it, and
// softens away once they rest.
func (a *app) immBar(c *ui.Context, tr *netease.Track, accent ui.Color, w float32) {
	pl := a.pl
	awake := a.npAwake(c)
	named := w >= immBarWide
	if awake {
		// Asleep again after the wait, a build turns the tools to their
		// hiding pose.
		c.After(npIdle - c.Now().Sub(a.npActiveAt) + 10*time.Millisecond)
	}
	reveal(c, ui.Column(c.Key("fc-bar")).Absolute().Left(0).Right(0).Bottom(18).Padding(0, 28).Gap(6), awake).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, clock(pl.Position())).FontSize(12).FontFeatures("tnum").TextColor(white.Alpha(0.55)).Width(40).TextAlign(ui.End)
			a.scrubber(c.Key("fc-scrub"), &a.coverScrub, white.Alpha(0.24), white.Alpha(0.92), 12, accent).Grow(1)
			ui.Text(c, clock(pl.Duration())).FontSize(12).FontFeatures("tnum").TextColor(white.Alpha(0.55)).Width(40)
		})
		ui.Row(c).Height(52).AlignItems(ui.Center).Gap(14).Children(func() {
			// The summary of what plays, at the left, and what one may
			// think of it; the words it takes have room, and give it up.
			ui.Row(c).Grow(1).Basis(0).MinWidth(120).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(44, 44).Radius(radiusTile).Children(func() { a.coverFill(c, tr.Album.PicURL, 120, radiusTile) })
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
				// The quality of the stream, and the song put in a playlist
				// from the page itself; an episode belongs to none.
				qualityBadge(c, pl.Stream())
				if tr.ProgramID == 0 {
					a.collectButton(c, tr)
				}
				a.paneButton(c, paneLyrics, iconLyrics, "歌词", true, named)
				a.paneButton(c, paneQueue, iconQueue, "待播清单", true, named)
				if a.countButton(c, paneComments, named).Clicked() {
					a.choosePane(paneComments)
				}
				// An episode has its description and its captions for the song's
				// lyrics and queue.
				if tr.ProgramID != 0 {
					a.paneButton(c, paneProgram, iconInfo, "简介", true, named)
					if a.captionsShown() {
						a.paneButton(c, paneCaptions, iconCaptions, "字幕", true, named)
					}
				}
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
