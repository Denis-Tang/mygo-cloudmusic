package app

import (
	"math"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/lyrics"
	"netease-music-mygo/internal/netease"
)

var white = ui.RGB(255, 255, 255)

var npSpring = ui.Animation{Spring: ui.SpringOptions{Duration: 450 * time.Millisecond, Bounce: 0.08}}

// npSettle is how long Now Playing takes to come or go, its spring at rest.
const npSettle = 600 * time.Millisecond

// npState tracks Now Playing opening and closing, and reports whether it is
// open and whether it has come to rest, open or closed. Call it once a
// build, before the window's content.
func (a *app) npState(c *ui.Context) (open, settled bool) {
	open = a.nowPlaying && a.pl.Current() != nil
	now := c.Now()
	if open != a.npOpen {
		a.npOpen, a.npChangedAt = open, now
		if open {
			// Shown, the page brings its tools with it: opening it by the
			// menu bar sends no pointer into the window.
			a.npActiveAt = now
		}
		// A build once it is at rest: to stop painting what it covers, or
		// itself once gone.
		c.After(npSettle)
		// The window buttons light over its dark backdrop, and as the
		// theme over the pages (Windows).
		if a.win != nil {
			buttons := mygo.ThemeSystem
			if open {
				buttons = mygo.ThemeDark
			}
			a.win.SetWindowButtonsTheme(buttons)
		}
	}
	return open, now.Sub(a.npChangedAt) >= npSettle
}

// nowPlayingLayer holds Now Playing over the whole window, rising into
// view as it opens and sinking away as it closes. The layer moves itself,
// staying on top all the way, rather than leaving as an exit transition
// would, under the rest of the window: that would show it going over the
// sidebar's material while the page above it hid it at once. Its pose
// moves it in frames that only paint, building nothing.
func (a *app) nowPlayingLayer(c *ui.Context, open, settled bool) {
	layer := ui.Box(c.Key("now-playing-layer")).Absolute().Top(0).Left(0).Right(0).Bottom(0)
	pose := ui.Pose{Y: 60, Fade: 1}
	if open {
		pose = ui.Pose{}
	}
	layer.AnimatePose(pose, npSpring)
	if !open && settled || a.pl.Current() == nil {
		// Closed: nothing to paint or click.
		layer.Invisible()
		a.queueAt = -1
		return
	}
	if !open {
		// On its way out, the window under it takes the clicks.
		layer.PassThrough()
	}
	layer.Children(func() { a.nowPlayingView(c) })
}

// nowPlayingView fills the window with what plays: its cover over a slow
// drift of its own colors, the controls, and lyrics that follow the song.
func (a *app) nowPlayingView(c *ui.Context) {
	pl := a.pl
	tr := pl.Current()
	w, h := c.Size()
	art := a.art.get(netease.Sized(tr.Album.PicURL, 600), true)
	// The colors stay the last song's while its cover loads, rather than
	// going gray between the two.
	shown := art
	if art == nil && tr.Album.PicURL != "" {
		shown = a.npArt
	}
	a.npArt = shown
	tint := ui.RGB(120, 120, 130)
	if shown != nil && !shown.failed {
		tint = shown.tint
	}
	accent := tint.Mix(white, 0.45)

	root := ui.Box(c.Key("now-playing")).Absolute().Top(0).Left(0).Right(0).Bottom(0).
		Background(ui.Hex("#0b0b0d")).TextColor(white)
	// Resting, the page hides the pointer with its controls, as a video
	// does: the pointer is the only thing left of the listener over the
	// picture.
	if !a.npAwake(c) {
		root.Cursor(ui.CursorNone)
	}
	root.Children(func() {
		a.backdrop(c, shown, tint)
		// Over the backdrop, which is positioned too: positioned elements
		// paint over those in the flow.
		ui.Column(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Children(func() { a.nowPlayingBody(c, tr, art, accent, w, h) })
	})
}

func (a *app) nowPlayingBody(c *ui.Context, tr *netease.Track, art *artwork, accent ui.Color, w, h float32) {
	a.commentsFor(tr)
	// An episode has what it is about and its captions in place of
	// lyrics; a song's lyrics hide until the picture or the bar asks.
	switch program := tr.ProgramID != 0; {
	case program && (a.npPane == paneLyrics || a.npPane == paneCover):
		a.npPane = paneProgram
		if a.captionsOn() {
			a.npPane = paneCaptions
		}
	case program && a.npPane == paneCaptions && !a.captionsShown():
		a.npPane = paneProgram
	case !program && (a.npPane == paneProgram || a.npPane == paneCaptions):
		a.npPane = paneCover
	}
	if a.npPane != paneQueue {
		a.queueAt = -1
	}
	// The page itself: the cover, the words, the readers and drawers, and
	// the bar. There is no other playing page's layout.
	awake := a.npAwake(c)
	a.immersive(c, tr, art, accent, w, h)
	// The top strip drags the window: over the picture, the way back to
	// the pages, at its left, and named, where the pointer finds it. It is
	// placed, and comes after the picture that it shows over. The pill goes
	// with the bar once the listener rests; the strip stays to drag. Resting,
	// the strip hides the pointer too, its clicks as the picture's.
	top := ui.Row(c).Absolute().Top(0).Left(0).Right(0).Height(topBar(c)).AlignItems(ui.Center).DragWindow()
	if !awake {
		top.Cursor(ui.CursorNone)
	}
	top.Children(func() {
		b := reveal(c, leaveButton(c, "返回"), awake)
		if !awake {
			b.Cursor(ui.CursorNone)
		}
		if b.Clicked() {
			a.nowPlaying = false
		}
	})
}

// leaveButton is the pill at the top left of the playing page, which goes
// back to the page under it.
func leaveButton(c *ui.Context, label string) ui.Element {
	b := ui.ButtonBase(c.Key("leave")).Height(32).PaddingX(12).Gap(4).Radius(999).FocusRing(false).
		AlignItems(ui.Center).Label(label).Margin(0, 0, 0, 14)
	bg := ui.RGBA(0, 0, 0, 0.22)
	if b.Hovered() {
		bg = ui.RGBA(0, 0, 0, 0.36)
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond})).TextColor(white)
	b.Children(func() {
		ui.Icon(c, iconDown).Size(15, 15)
		ui.Text(c, label).FontSize(13).FontWeight(600).TextColor(white)
	})
	return b
}

// paneButton is one of the buttons over the top of Now Playing, which open
// a pane of it, or, paneCover, return to the cover: lit where shown, gone
// in a pane's own view. Songs and episodes the icons differ of, and an
// episode's buttons the labels of, show when ok is set for: captions only
// once the model is there, and its comments by its tab. A bar too narrow
// to name them shows their icons alone.
func (a *app) paneButton(c *ui.Context, pane int, icon *ui.SVG, label string, ok, named bool) {
	if !ok {
		return
	}
	at := a.npPane == pane
	// The button's name names where it goes: the panes' names are other
	// elements' too, as the song's menu's items are.
	b := ui.ButtonBase(c.Key(label)).Padding(6, 14).Radius(999).FocusRing(false).Label(label + "面板").Gap(6)
	if !named {
		b.Padding(6, 10)
	}
	bg, fg := white.Alpha(0.06), white.Alpha(0.6)
	if at {
		bg, fg = white.Alpha(0.14), white
	} else if b.Hovered() {
		fg = white
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond})).TextColor(fg)
	b.Children(func() {
		ui.Icon(c, icon).Size(13, 13)
		if named {
			ui.Text(c, label).FontSize(13).FontWeight(600).TextColor(fg)
		}
	})
	if b.Clicked() {
		a.choosePane(pane)
	}
}

// countButton is the comments' pane's button: its count, kept rolling as
// one song's gives to the next, in the wheels of a counter.
func (a *app) countButton(c *ui.Context, pane int, named bool) ui.Element {
	b := ui.ButtonBase(c.Key("评论面板")).Padding(6, 14).Radius(999).FocusRing(false).Label("评论面板").Gap(6)
	if !named {
		b.Padding(6, 10)
	}
	bg, fg := white.Alpha(0.06), white.Alpha(0.6)
	if a.npPane == pane {
		bg, fg = white.Alpha(0.14), white
	} else if b.Hovered() {
		fg = white
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond})).TextColor(fg)
	b.Children(func() {
		ui.Icon(c, iconComment).Size(13, 13)
		if named {
			ui.Text(c, "评论").FontSize(13).FontWeight(600).TextColor(fg)
		}
		// The count rolls as it changes: kept, where its tab kept it.
		n, text := a.commentCount(), ""
		if n > 0 {
			text = " " + count(n)
			b.Label("评论面板 · " + count(n))
		}
		rollCount(c, &a.tabCount, n, text, fg)
	})
	return b
}

// reveal fades e in while the listener is at the playing page and out once
// they rest, and, the fade over, leaves it taking nothing, so that a click
// where it was falls on the picture under it. Only the listener wakes it:
// the page's own frames, drawn for the words as they are sung, must not.
func reveal(c *ui.Context, e ui.Element, shown bool) ui.Element {
	to := float32(0)
	if shown {
		to = 1
	}
	fade := e.AnimateTo("shown", to, ui.Animation{Duration: 200 * time.Millisecond})
	e.Opacity(fade)
	if !shown && fade < 0.02 {
		e.Invisible()
	}
	return e
}

// lyricsPaneChosen is the pane of lyrics the listener chose last: what
// the app starts on, before the player is made. Now Playing shows an
// episode's in place of either.
func (a *app) lyricsPaneChosen() int {
	return paneCover
}

// The panes of Now Playing. The cover comes first, as it is the view the
// app shows: a pane opens over it, and the app starts on the cover again.
const (
	paneCover = iota
	paneLyrics
	paneQueue
	paneComments
	paneProgram
	paneCaptions
)

// choosePane shows a pane of Now Playing, or, paneCover, the cover.
func (a *app) choosePane(i int) {
	a.npPane = i
}

// radioChip says which radio plays, over the title: a click on it, or on
// its cross, leaves the radio, playing on the song.
func (a *app) radioChip(c *ui.Context, name string) {
	icon, exit := iconRadio, "退出私人 FM"
	if name == heartRadio {
		icon, exit = iconHeartFill, "退出心动模式"
	}
	chip := ui.ButtonBase(c.Key("radio-chip")).Gap(5).Padding(3, 6, 3, 8).Margin(0, 0, 4, -8).Radius(999).
		AlignItems(ui.Center).AlignSelf(ui.Start).FocusRing(false).Label(exit).Tooltip(exit)
	bg, fg, cross := ui.Transparent, white.Alpha(0.6), float32(0.5)
	if chip.Hovered() {
		bg, fg, cross = white.Alpha(0.12), white.Alpha(0.85), 1
	}
	chip.Background(chip.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond})).TextColor(fg)
	chip.Children(func() {
		ui.Icon(c, icon).Size(12, 12)
		ui.Text(c, radioLabel(name)).FontSize(11.5).FontWeight(700).LetterSpacing(0.3)
		ui.Icon(c, iconX).Size(11, 11).Opacity(cross)
	})
	if chip.Clicked() {
		a.exitRadio()
	}
}

// qualityBadge marks a lossless stream, as it plays.
func qualityBadge(c *ui.Context, st *netease.Stream) {
	if st == nil || !netease.Lossless(st.Level) {
		return
	}
	label := "无损"
	switch st.Level {
	case "hires":
		label = "Hi-Res"
	case "jymaster":
		label = "母带"
	}
	ui.Text(c, label).FontSize(9.5).FontWeight(800).LetterSpacing(0.6).TextColor(ui.Hex("#f5d58a")).
		Padding(1, 6).Radius(4).Border(1, ui.Hex("#f5d58a").Alpha(0.55))
}

// stageBar is gone with the stage: Now Playing's controls live under the
// cover, in every pane.

func (a *app) lyricsPane(c *ui.Context, tr *netease.Track, h float32) {
	pl := a.pl
	lines, loaded := pl.Lyrics()
	if len(lines) == 0 {
		col := ui.Column(c).Grow(1)
		if loaded {
			col.Center()
		}
		dim, _ := a.quietShown(c, col, tr, lines, loaded)
		quietUnder(col, dim)
		col.Children(func() {
			if !loaded {
				lyricsSkeleton(c, npShimmer(c))
				return
			}
			ui.Text(c, "纯音乐，请欣赏").FontSize(24).FontWeight(700).TextColor(white.Alpha(0.4))
		})
		a.quietCard(c, dim, h*0.18)
		return
	}
	pos := pl.Position()
	cur := lyrics.Index(lines, pos)
	// Redraw as the next line begins.
	if pl.Playing() && cur+1 < len(lines) {
		c.After(lines[cur+1].At - pos + 20*time.Millisecond)
	}
	if a.lyricTrack != tr.ID {
		a.lyricTrack, a.lyricAt = tr.ID, -2
		a.lyricScroll, a.lyricGlide = ui.ScrollState{}, lyricGlide{}
	}
	lag := a.lyricGlide.step(c, &a.lyricScroll)

	scroll := ui.Scroll(c.Key("lyrics")).Grow(1).TrackScroll(&a.lyricScroll).Padding(0, 8, h*0.5, 0)
	dim, quiet := a.quietShown(c, scroll, tr, lines, loaded)
	if dim > 0.98 {
		// Under the hot comment, gone, not drawn faint.
		scroll.Invisible()
	}
	defer a.quietCard(c, dim, h*0.18)
	a.timedLines(c, scroll, lines, pos, cur, lag, linesStyle{size: 30, weight: 800, lineHeight: 1.25, quiet: quiet}, nil)
}

// linesStyle is how timedLines sets its lines: the size, weight and
// leading of their type; quiet nearly hides them, under a hot comment.
type linesStyle struct {
	size       float32
	weight     int
	lineHeight float32
	quiet      bool
}

// timedLines fills scroll with lines that follow the track playing at
// pos: the line at cur lit, and brought to a third of the way down once
// it changes, but none with cur -1 before the first; a click on one seeks
// to it. The lines are held back by lag, how far the glide to the line
// lit has still to go (lyricGlide). foot, if set, follows the lines.
func (a *app) timedLines(c *ui.Context, scroll ui.Element, lines []lyrics.Line, pos time.Duration, cur int, lag float32, st linesStyle, foot func()) {
	pl := a.pl
	var active ui.Element
	scroll.Children(func() {
		ui.Column(c).Gap(6).Translate(0, -lag).Children(func() {
			for i, l := range lines {
				line := ui.ButtonBase(c.Key(i)).Column().AlignItems(ui.Start).Padding(10, 12).Radius(12).Gap(6).FocusRing(false).Label(l.Text)
				on := i == cur
				alpha := float32(0.28)
				if on {
					alpha = 1
				} else if line.Hovered() {
					alpha = 0.55
				}
				if st.quiet {
					// Nearly gone under a hot comment.
					alpha *= quietFaded
				}
				if line.Hovered() {
					line.Background(white.Alpha(0.06))
				}
				scale := float32(0.94)
				if on {
					scale = 1
				}
				line.TransformOrigin(0, 0.5).AnimatePose(ui.Pose{Scale: scale}, ui.Animation{Spring: ui.SpringOptions{Duration: 600 * time.Millisecond, Bounce: 0.1}})
				line.Children(func() {
					// The line fades as a pose, in frames that only paint, rather
					// than building the view at the display's rate as it does.
					text := ui.Column(c).AlignItems(ui.Start).Gap(6)
					text.AnimatePose(ui.Pose{Fade: 1 - alpha}, ui.Animation{Duration: 380 * time.Millisecond})
					text.Children(func() {
						if on && len(l.Words) > 0 {
							// The line sung lights up word by word as it is sung.
							ui.RichText(c.Key("words")).FontSize(st.size).FontWeight(st.weight).LineHeight(st.lineHeight).LetterSpacing(-0.4).Children(func() {
								for wi, wd := range l.Words {
									lit := clamp01(float32((pos - wd.Start).Seconds() / max(0.05, (wd.End-wd.Start).Seconds())))
									ui.Text(c.Key(wi), wd.Text).TextColor(white.Alpha(0.38 + 0.62*lit))
								}
							})
							if pl.Playing() {
								// Words brighten smoothly enough at 25 frames a second.
								c.After(40 * time.Millisecond)
							}
						} else {
							ui.Text(c, l.Text).FontSize(st.size).FontWeight(st.weight).LineHeight(st.lineHeight).LetterSpacing(-0.4).TextColor(white)
						}
						if l.Translation != "" {
							ui.Text(c, l.Translation).FontSize(17).FontWeight(600).TextColor(white.Alpha(0.7))
						}
					})
				})
				if line.Clicked() {
					pl.Seek(l.At)
				}
				if on {
					active = line
				}
			}
		})
		if foot != nil {
			foot()
		}
	})
	// Bring the line sung to a third of the way down, once it changed.
	if cur != a.lyricAt {
		box := scroll.Bounds()
		target := float32(0)
		if active.Valid() {
			// Where the glide is going, without where it still is.
			if lb := active.LayoutBounds(); lb.H > 0 && box.H > 0 {
				target = lb.Y - box.Y + a.lyricScroll.Y - box.H*0.3
			} else {
				return // not laid out yet: the next frame scrolls
			}
		}
		a.lyricAt = cur
		a.lyricGlide.to(c, &a.lyricScroll, min(max(0, target), a.lyricScroll.MaxY))
	}
}

// lyricGlide scrolls the lyrics to the line sung, as a critically damped
// spring, as ScrollTo's would. ScrollTo moves the offset in every frame,
// and MyGo builds the view again in the frame after each that moved it,
// and its spring creeps over the last pixel for half a second: with a line
// following another every second or two, most of a song. So the offset
// goes to the line at once, in one frame, and the lines are held back by
// what the glide has still to go, its lag, which settles to nothing within
// half a pixel.
type lyricGlide struct {
	on bool
	// The lag t after start: (a + b·t)·e^(−ωt), in DIPs.
	a, b  float64
	start time.Time
	// The offset it goes to, set in the next frame (pending), with the lag
	// that holds the lines back from it: the layout of the frame that
	// chose it takes the offset too, the lines built without the lag.
	target  float32
	pending bool
	set     float32 // the offset it set
}

// lyricOmega is the spring's ω: a line or two away, it comes within half a
// pixel in about 800 ms.
const lyricOmega = 10

// at returns the lag at now, and how fast it changes, in DIPs a second.
func (g *lyricGlide) at(now time.Time) (lag, v float64) {
	if !g.on {
		return 0, 0
	}
	t := now.Sub(g.start).Seconds()
	e := math.Exp(-lyricOmega * t)
	return (g.a + g.b*t) * e, (g.b - lyricOmega*(g.a+g.b*t)) * e
}

// to scrolls s to target from the next frame, gliding from where it
// shows, at the speed it went if it was on its way.
func (g *lyricGlide) to(c *ui.Context, s *ui.ScrollState, target float32) {
	now := c.Now()
	lag, v := g.at(now)
	shown := float64(s.Y) + lag
	g.target, g.pending = target, true
	g.on = !c.Preferences().ReduceMotion
	g.start, g.a = now, shown-float64(target)
	g.b = v + lyricOmega*g.a
	c.AnimationFrame()
}

// step returns the lag of the lines in this frame, and asks for the next
// while there is one. The listener scrolling stops the glide where it
// shows.
func (g *lyricGlide) step(c *ui.Context, s *ui.ScrollState) float32 {
	if g.pending {
		s.Y, g.set, g.pending = g.target, g.target, false
	}
	if !g.on {
		return 0
	}
	lag, v := g.at(c.Now())
	if s.Y != g.set {
		// The listener took over: the offset takes what showed.
		s.Y, g.on = max(0, min(s.Y+float32(lag), s.MaxY)), false
		return 0
	}
	if math.Abs(lag) < 0.5 && math.Abs(v) < 20 {
		g.on = false
		return 0
	}
	c.AnimationFrame()
	return float32(lag)
}

// queuePane lists the whole queue in play order, the track playing marked
// among the others: a click plays one there, and the list stays as it is.
func (a *app) queuePane(c *ui.Context) {
	pl := a.pl
	order, at := pl.PlayOrder()
	// Opened, it shows the track playing at its top; after, it follows it
	// as little as keeps it in view.
	if at >= 0 && at != a.queueAt {
		if a.queueAt < 0 {
			a.queueList.ScrollTo(at, ui.Start)
		} else {
			a.queueList.ScrollIntoView(at)
		}
		a.queueAt = at
	}
	a.queueList.Key = func(row int) any { return order[row] }
	ui.List(c.Key("queue"), &a.queueList, len(order), func(r int) {
		i := order[r]
		tr := &pl.Queue()[i]
		playing := r == at
		row := ui.ButtonBase(c.Key(i)).Height(58).PaddingX(10).Gap(14).Radius(12).AlignItems(ui.Center).FocusRing(false).Label(tr.Name).
			ContextMenu(a.songItems(tr, 0))
		switch {
		case playing:
			row.Background(white.Alpha(0.12))
		case row.Hovered():
			row.Background(white.Alpha(0.08))
		}
		if !tr.Playable() {
			row.Opacity(0.4)
		}
		row.Children(func() {
			a.cover(c, tr.Album.PicURL, 96, 42, 7)
			ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
				ui.Text(c, tr.Name).FontSize(14).FontWeight(600).SingleLine()
				a.artistLinks(c, tr, white.Alpha(0.55), white).FontSize(12.5).SingleLine()
			})
			ui.Text(c, clock(tr.Length())).FontSize(12).FontFeatures("tnum").TextColor(white.Alpha(0.4))
		})
		if row.Clicked() && tr.Playable() && !playing {
			pl.Jump(i)
		}
	}).Grow(1).Padding(8, 0, 40, 0).Gap(2)
}
