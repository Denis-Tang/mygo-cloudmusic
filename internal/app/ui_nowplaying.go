package app

import (
	"math"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/shader"

	"netease-music-mygo/internal/lyrics"
	"netease-music-mygo/internal/netease"
)

var white = ui.RGB(255, 255, 255)

// npHeroSpring is how the cover goes from the bar to the page, and back:
// a spring that keeps its speed as the listener changes their mind.
var npHeroSpring = ui.Animation{Spring: ui.SpringOptions{Duration: 520 * time.Millisecond, Bounce: 0.14}}

// npContent is how the page's words and tools come in after the cover, and
// go before it does.
var npContent = ui.Animation{Spring: ui.SpringOptions{Duration: 320 * time.Millisecond, Bounce: 0.04}, Delay: 100 * time.Millisecond}

// npBackdrop is how the cover's colors come under the page, and go.
var npBackdrop = ui.Animation{Duration: 240 * time.Millisecond}

// npSettle is how long Now Playing takes to come or go, its cover at rest.
const npSettle = 650 * time.Millisecond

// npHeroMatch ties the cover of the bar to the cover of the page, which
// grows from it as the page opens.
type npHeroMatch struct{ id int64 }

// npCoverSize is the cover of the bar's, which the page's comes from.
const npCoverSize = 48

// npState tracks Now Playing opening and closing, and reports whether it is
// open and whether it has come to rest, open or closed. Call it once a
// build, before the window's content.
func (a *app) npState(c *ui.Context) (open, settled bool) {
	open = a.nowPlaying && a.pl.Current() != nil
	now := c.Now()
	if open != a.npOpen {
		// From rest, the page's cover grows from the bar's. Turned again
		// before it was at rest, it keeps the turn it had: no jump back to
		// the bar cover, which the spring would take as a new start.
		a.npHeroFromBar = open && a.npSettled
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
	settled = now.Sub(a.npChangedAt) >= npSettle
	a.npSettled = settled
	return open, settled
}

// nowPlayingLayer holds Now Playing over the whole window. The page's cover
// grows from the bar's, the words follow it, and on the way out the cover
// goes back to the bar before the page is gone: a shared element, as
// Apple's Music opens its player, rather than a sheet sliding up. The
// layer stays on top all the way, building only what still shows.
func (a *app) nowPlayingLayer(c *ui.Context, open, settled bool) {
	layer := ui.Box(c.Key("now-playing-layer")).Absolute().Top(0).Left(0).Right(0).Bottom(0)
	if !open && settled || a.pl.Current() == nil {
		// Closed: nothing to paint or click. The page keeps its root, empty,
		// that what comes with the next opening has a parent to enter.
		layer.Invisible()
		a.queueAt = -1
		if a.pl.Current() != nil {
			layer.Children(func() { a.nowPlayingView(c, false, true) })
		}
		return
	}
	if !open {
		// On its way out, the window under it takes the clicks.
		layer.PassThrough()
	}
	layer.Children(func() { a.nowPlayingView(c, open, settled) })
}

// nowPlayingView fills the window with what plays: the cover over a slow
// drift of its own colors, the controls, and lyrics that follow the song.
// Opening, the cover is the bar's, grown; the words and tools follow it in.
// Closing, the cover goes back to the bar while the words fade behind it.
func (a *app) nowPlayingView(c *ui.Context, open, settled bool) {
	if !open && settled {
		// Closed: the page's root alone, that what comes with the next
		// opening has a parent to enter. Nothing of it is built or fetched.
		ui.Box(c.Key("now-playing")).Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough()
		return
	}
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
	hue, saturation, _ := hsv(float64(tint.R)/255, float64(tint.G)/255, float64(tint.B)/255)
	accent := hsvColor(hue, min(saturation, 0.28), 0.96)

	root := ui.Box(c.Key("now-playing")).Absolute().Top(0).Left(0).Right(0).Bottom(0).TextColor(white)
	if !open {
		root.PassThrough()
	}
	accent = root.AnimateColor("cover-accent", accent, ui.Snappy)
	// Resting, the page hides the pointer with its controls, as a video
	// does: the pointer is the only thing left of the listener over the
	// picture.
	if !a.npAwake(c) {
		root.Cursor(ui.CursorNone)
	}
	root.Children(func() {
		if open {
			// The colors behind the cover, which keep the words readable
			// before it has painted and after it goes.
			bd := ui.Box(c.Key("np-backdrop")).Absolute().Top(0).Left(0).Right(0).Bottom(0).
				Background(ui.Hex("#0b0b0d"))
			bd.Transition(ui.ElementTransition{Animation: &npBackdrop, Enter: &ui.Motion{}, Exit: &ui.Motion{}})
			bd.Children(func() { a.backdrop(c, shown, tint) })
		}
		// The cover, over the colors and under the words: the bar's grown
		// to the page, and, closing, the page's gone back to the bar.
		a.npHero(c, tr, art, open, settled)
		if open {
			// The words, the readers and the bar: after the cover, before it
			// goes, and moved a little as they come and go.
			body := ui.Box(c.Key("np-content")).Absolute().Top(0).Left(0).Right(0).Bottom(0)
			body.Transition(ui.ElementTransition{Animation: &npContent, Enter: &ui.Motion{Y: 10}, Exit: &ui.Motion{Y: 8}})
			body.Children(func() { a.nowPlayingBody(c, tr, accent, w, h) })
		}
	})
}

// npHero draws the cover of the page: the bar's cover, grown to the whole
// window as the page opens, and, closing, shrinking back to the bar. While
// it comes from the bar, a shared element takes it there; on the way back
// it goes to the bar's own box, so it stays over the page's words to the
// end.
func (a *app) npHero(c *ui.Context, tr *netease.Track, art *artwork, open, settled bool) {
	sharp := bitmap(a.art.get(netease.Sized(tr.Album.PicURL, 1536), false))
	if sharp == nil {
		sharp = bitmap(art)
	}
	hero := ui.Box(c.Key("np-hero")).Radius(radiusTile).Clip().PassThrough().Label("播放页封面")
	if open {
		hero.Absolute().Top(0).Left(0).Right(0).Bottom(0)
		t := ui.ElementTransition{Animation: &npHeroSpring, Position: true, Size: true}
		if !settled && a.npHeroFromBar {
			t.Match = npHeroMatch{tr.ID}
		}
		hero.Transition(t)
	} else {
		x, y := a.npCoverOrigin(c)
		hero.Absolute().Left(x).Top(y).Width(npCoverSize).Height(npCoverSize)
		hero.Transition(ui.ElementTransition{Animation: &npHeroSpring, Position: true, Size: true})
	}
	hero.Draw(func(p *ui.Painter, r ui.Rect) {
		if sharp == nil {
			return
		}
		soft := sharp
		if art != nil && art.stage != nil {
			soft = art.stage
		}
		blendShader().Draw(p, r, shader.Options{Images: [4]*ui.Bitmap{sharp, soft}})
	})
}

// npCoverOrigin is where the bar's cover sits in the window, which the
// page's cover goes back to.
func (a *app) npCoverOrigin(c *ui.Context) (float32, float32) {
	// The bar runs along the whole window, under the sidebar too: its
	// cover sits at its own left padding and its row's, not the content's.
	const pad = 8
	_, h := c.Size()
	top := h - playerBarHeight + pad + (playerBarHeight-pad-npCoverSize)/2
	return 28, top
}

func (a *app) nowPlayingBody(c *ui.Context, tr *netease.Track, accent ui.Color, w, h float32) {
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
	// A song's queue and comments are the bar's [...] panel: the pane that
	// stands for them is the panel open, which its entry toggles, and a
	// press outside it, or the escape key, closes. An episode has its
	// program's drawers in their place.
	if tr.ProgramID == 0 {
		a.npDrawer.open = a.npPane == paneQueue || a.npPane == paneComments
	} else {
		a.npDrawer.open = false
	}
	// The page itself: the cover, the words, the readers and drawers, and
	// the bar. There is no other playing page's layout.
	awake := a.npAwake(c)
	a.immersive(c, tr, accent, w, h)
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
	b := ui.ButtonBase(c.Key("leave")).Size(40, 40).Radius(radiusPill).FocusRing(false).
		Center().Label(label).Margin(0, 0, 0, 14)
	bg := ui.RGBA(0, 0, 0, 0.28)
	if b.Hovered() {
		bg = ui.RGBA(0, 0, 0, 0.42)
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond})).
		Border(1, white.Alpha(0.1)).TextColor(white)
	b.Children(func() {
		ui.Icon(c, iconDown).Size(22, 22)
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
	b, fg := barButton(c, label, named, at, label+"面板")
	b.Children(func() {
		ui.Icon(c, icon).Size(18, 18)
		if named {
			ui.Text(c, label).FontSize(16).FontWeight(600).TextColor(fg)
		}
	})
	if b.Clicked() {
		a.choosePane(pane)
	}
}

// barButton is a button of the playing page's bar: round, as tall as the
// others there, its name's letters when the bar has room for them, and a
// soft disc under the pointer, lit while it stands for the pane showing.
// The bar's entries are all of it, whatever they say, so that they sit on
// one line and keep one height.
func barButton(c *ui.Context, key any, named, lit bool, label string) (ui.Element, ui.Color) {
	b := ui.ButtonBase(c).Key(key).Height(32).Center().Radius(radiusPill).FocusRing(false).Label(label)
	if named {
		b.PaddingX(12).Gap(6)
	} else {
		b.Width(36)
	}
	bg, fg := white.Alpha(0), white.Alpha(0.6)
	if lit {
		bg, fg = white.Alpha(0.14), white
	} else if b.Hovered() {
		bg, fg = white.Alpha(0.08), white
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond})).TextColor(fg)
	return b, fg
}

// drawerButton is the [...] of the bar, which opens what aids the playing
// page: the queue and the comments, at once, in a panel over the middle of
// the page, which a click on it again closes. The panel is built with the
// rest of the page, that its box is placed against the window.
func (a *app) drawerButton(c *ui.Context, tr *netease.Track, named bool) {
	open := a.npPane == paneQueue || a.npPane == paneComments
	label := "更多"
	if open {
		label = "收起待播清单与评论"
	}
	b, fg := barButton(c, "more", named, open, label)
	b.Children(func() {
		ui.Icon(c, iconMore).Size(18, 18)
		if named {
			ui.Text(c, "更多").FontSize(16).FontWeight(600).TextColor(fg)
		}
	})
	if b.Clicked() {
		if open {
			a.choosePane(paneCover)
		} else {
			a.choosePane(paneQueue)
		}
	}
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
	chip := ui.ButtonBase(c.Key("radio-chip")).Gap(5).Padding(3, 6, 3, 8).Margin(0, 0, 4, -8).Radius(radiusPill).
		AlignItems(ui.Center).AlignSelf(ui.Start).FocusRing(false).Label(exit).Tooltip(exit)
	bg, fg, cross := ui.Transparent, white.Alpha(0.6), float32(0.5)
	if chip.Hovered() {
		bg, fg, cross = white.Alpha(0.12), white.Alpha(0.85), 1
	}
	chip.Background(chip.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond})).TextColor(fg)
	chip.Children(func() {
		ui.Icon(c, icon).Size(12, 12)
		ui.Text(c, radioLabel(name)).FontSize(14).FontWeight(700).LetterSpacing(0.3)
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
		Padding(1, 6).Radius(radiusTile-2).Border(1, ui.Hex("#f5d58a").Alpha(0.55))
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
				line := ui.ButtonBase(c.Key(i)).Column().AlignItems(ui.Start).Padding(10, 12).Radius(radiusRow).Gap(6).FocusRing(false).Label(l.Text)
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
// Its rows are laid out one under the other, in the panel that scrolls
// them: a list of its own inside would scroll twice.
func (a *app) queuePane(c *ui.Context) {
	pl := a.pl
	order, at := pl.PlayOrder()
	ui.Column(c).Gap(2).Children(func() {
		for r, i := range order {
			tr := &pl.Queue()[i]
			playing := r == at
			row := ui.ButtonBase(c.Key(i)).Height(52).PaddingX(10).Gap(12).Radius(radiusRow).AlignItems(ui.Center).FocusRing(false).Label(tr.Name)
			bg := ui.Transparent
			switch {
			case playing:
				bg = white.Alpha(0.12)
			case row.Hovered():
				bg = white.Alpha(0.08)
			}
			row.Background(row.AnimateColor("bg", bg, hoverFade))
			if !tr.Playable() {
				row.Opacity(0.4)
			}
			row.Children(func() {
				a.cover(c, tr.Album.PicURL, 96, 34, radiusTile)
				ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
					ui.Text(c, tr.Name).FontSize(17).FontWeight(600).SingleLine()
					a.artistLinks(c, tr, white.Alpha(0.55), white).FontSize(14).SingleLine()
				})
				ui.Text(c, clock(tr.Length())).FontSize(14).FontFeatures("tnum").TextColor(white.Alpha(0.4))
				a.songMenu(c, row, tr, 0, white.Alpha(0.6))
			})
			if row.Clicked() && tr.Playable() && !playing {
				pl.Jump(i)
			}
		}
	})
}
