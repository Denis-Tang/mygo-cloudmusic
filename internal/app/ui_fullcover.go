package app

import (
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/lyrics"
	"netease-music-mygo/internal/netease"
)

// fullCover is the cover on the whole screen, in a window of its own: the
// cover as big as the screen fits it, over the colors of what plays, the
// lyrics hidden until a click asks for them, the controls shown while the
// pointer is in the window. The listener leaves it with the escape key,
// and the cover's click shows and hides the lyrics.
func (a *app) openFullCover() {
	if a.pl.Current() == nil {
		return
	}
	if a.fullCoverWin == nil {
		a.fullCoverWin = mygo.NewWindow(mygo.WindowOptions{
			Title:           mygo.App.Name(),
			FullScreen:      true,
			DisableMinimize: true,
			AutoHideMenuBar: true,
			BackgroundColor: "#0b0b0d",
			Content:         ui.View(a.fullCoverView),
		})
		a.fullCoverWin.OnClosed(func() {
			a.fullCoverWin = nil
			a.fcLyrics = false
			a.syncDock()
		})
		a.syncDock()
	}
	a.fullCoverWin.Show()
	a.fullCoverWin.Focus()
}

// closeFullCover leaves the fullscreen cover.
func (a *app) closeFullCover() {
	if a.fullCoverWin != nil {
		a.fullCoverWin.Close()
	}
}

// fullCoverView fills the fullscreen window with what plays.
func (a *app) fullCoverView(c *ui.Context) {
	pl := a.pl
	c.OnShortcut(0, ui.KeyEscape, a.closeFullCover)
	tr := pl.Current()
	w, h := c.Size()
	if tr == nil {
		// The song went, as the queue emptied, before the window closes.
		ui.Column(c).Grow(1).Center().Children(func() {
			ui.Text(c, "没有正在播放的音乐").FontSize(15).TextColor(white.Alpha(0.5))
		})
		return
	}
	art := a.art.get(netease.Sized(tr.Album.PicURL, 900), true)
	// The colors stay the last song's while its cover loads, as Now
	// Playing's keep.
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

	// The square the height leaves room for, over the controls.
	size := min(w*0.52, h-440)
	if size < 220 {
		size = w * 0.52
	}

	root := ui.Box(c.Key("full-cover")).Absolute().Top(0).Left(0).Right(0).Bottom(0).
		Background(ui.Hex("#0b0b0d")).TextColor(white)
	root.Children(func() {
		a.backdrop(c, shown, tint)
		// The cover, on its own in the view, its click the lyrics' way
		// in and out.
		b := ui.ButtonBase(c.Key("fc-cover")).AlignSelf(ui.Center).Radius(24).FocusRing(false).
			Label(coverLabel(a.fcLyrics))
		b.Children(func() {
			a.cover(c, tr.Album.PicURL, 900, size, 24)
		})
		if b.Clicked() {
			a.fcLyrics = !a.fcLyrics
		}
		// The controls and the exit, showing while the pointer is in.
		pointerIn := c.Root().Hovered()
		reveal(ui.Column(c.Key("fc-bar")).Absolute().Left(0).Right(0).Bottom(28).Padding(0, h*0.2).Gap(14), pointerIn).Children(func() {
			ui.Row(c).Justify(ui.Center).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Text(c, tr.Name).FontSize(16).FontWeight(700).SingleLine()
				a.likeButton(c.Key("bar-like"), tr, 15, white.Alpha(0.5))
				a.artistLinks(c, tr, white.Alpha(0.55), white).FontSize(12).SingleLine()
			})
			a.scrubber(c.Key("fc-scrub"), &a.fcScrub, white.Alpha(0.15), white.Alpha(0.9), 12, accent.Mix(white, 0.35))
			ui.Row(c).Justify(ui.Center).Children(func() {
				a.transport(c, white, white.Alpha(0.45), accent, 24)
			})
		})
		reveal(ui.Row(c.Key("fc-top")).Absolute().Top(14).Right(28).AlignItems(ui.Center).Gap(6), pointerIn).Children(func() {
			a.songMenu(c.Key("more"), tr, 20, white.Alpha(0.7))
			if iconButton(c.Key("exit"), iconX, 20, white.Alpha(0.7), "退出全屏封面").Clicked() {
				a.closeFullCover()
			}
		})
		// The lyrics over the cover, once a click asked for them.
		if a.fcLyrics {
			ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough().Children(func() {
				a.fcLyricPane(c, tr, w, h)
			})
		}
	})
}

// fullCoverButton is the button of Now Playing's top strip that opens the
// fullscreen cover, in the style of the panes' buttons, apart from them.
func (a *app) fullCoverButton(c *ui.Context) ui.Element {
	b := ui.ButtonBase(c.Key("full-cover")).Padding(6, 14).Radius(999).FocusRing(false).Label("全屏封面").Gap(6)
	bg, fg := white.Alpha(0.06), white.Alpha(0.6)
	if b.Hovered() {
		bg, fg = white.Alpha(0.12), white
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond})).TextColor(fg)
	b.Children(func() {
		ui.Icon(c, iconFull).Size(13, 13)
		ui.Text(c, "全屏封面").FontSize(13).FontWeight(600).TextColor(fg)
	})
	return b
}

// coverLabel says what the cover's click does now.
func coverLabel(lyricsShown bool) string {
	if lyricsShown {
		return "隐藏歌词，回到封面"
	}
	return "显示歌词"
}

// fcLyricPane is the song's lyrics, over the cover, following it as Now
// Playing's lyrics follow it. Its scroll and glide are the fullscreen
// window's, kept apart from the main window's, where Now Playing keeps
// its own.
func (a *app) fcLyricPane(c *ui.Context, tr *netease.Track, w, h float32) {
	pl := a.pl
	lines, _ := pl.Lyrics()
	if len(lines) == 0 {
		return
	}
	pos := pl.Position()
	cur := lyrics.Index(lines, pos)
	// Redraw as the next line begins.
	if pl.Playing() && cur+1 < len(lines) {
		c.After(lines[cur+1].At - pos + 20*time.Millisecond)
	}
	if a.fcTrack != tr.ID {
		a.fcTrack, a.fcAt = tr.ID, -2
		a.fcScroll, a.fcGlide = ui.ScrollState{}, lyricGlide{}
	}
	lag := a.fcGlide.step(c, &a.fcScroll)
	scroll := ui.Scroll(c.Key("fc-lyrics")).Grow(1).TrackScroll(&a.fcScroll).
		Width(min(w*0.6, 620)).Padding(0, 8, h*0.4, 0)
	a.fcLines(c, scroll, lines, pos, cur, lag)
}

// fcLines fills scroll with the lyrics that follow what plays: the line
// sung lit, and brought to a third of the way down once it changes; a
// click on one seeks to it. The scrolling and where the line got to are
// kept apart: a.fcAt, a.fcGlide and fcLines' scroll see only this pane.
func (a *app) fcLines(c *ui.Context, scroll ui.Element, lines []lyrics.Line, pos time.Duration, cur int, lag float32) {
	pl := a.pl
	var active ui.Element
	scroll.Children(func() {
		ui.Column(c).Gap(8).Translate(0, -lag).Children(func() {
			for i, l := range lines {
				line := ui.ButtonBase(c.Key(i)).Column().AlignItems(ui.Start).Padding(10, 12).Radius(12).Gap(6).FocusRing(false).Label(l.Text)
				alpha := float32(0.3)
				if i == cur {
					alpha = 1
				} else if line.Hovered() {
					alpha = 0.55
				}
				text := ui.Column(c).AlignItems(ui.Start)
				text.AnimatePose(ui.Pose{Fade: 1 - alpha}, ui.Animation{Duration: 380 * time.Millisecond})
				line.Children(func() {
					text.Children(func() {
						if i == cur && len(l.Words) > 0 {
							ui.RichText(c).FontSize(34).FontWeight(800).LineHeight(1.3).LetterSpacing(-0.4).Children(func() {
								for wi, wd := range l.Words {
									lit := clamp01(float32((pos - wd.Start).Seconds() / max(0.05, (wd.End-wd.Start).Seconds())))
									ui.Text(c.Key(wi), wd.Text).TextColor(white.Alpha(0.38 + 0.62*lit))
								}
							})
							if pl.Playing() {
								c.After(40 * time.Millisecond)
							}
						} else {
							ui.Text(c, l.Text).FontSize(34).FontWeight(800).LineHeight(1.3).LetterSpacing(-0.4)
						}
						if l.Translation != "" {
							ui.Text(c, l.Translation).FontSize(19).FontWeight(600).TextColor(white.Alpha(0.7))
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
	if cur != a.fcAt {
		box := scroll.Bounds()
		target := float32(0)
		if active.Valid() {
			if lb := active.LayoutBounds(); lb.H > 0 && box.H > 0 {
				target = lb.Y - box.Y + a.fcScroll.Y - box.H*0.3
			} else {
				return // not laid out yet: the next frame scrolls
			}
		}
		a.fcAt = cur
		a.fcGlide.to(c, &a.fcScroll, min(max(0, target), a.fcScroll.MaxY))
	}
}
