package app

import (
	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// A song's collection action lists the playlists it may be added to: the
// playlists the listener made, the liked songs first among them, what each
// holds, and a way to make another. It is dark over the picture whatever
// the appearance of the pages, as the queue and the comments are.

// collectDialog is the song to put in a playlist,
// and where its list is scrolled.
type collectDialog struct {
	open    bool
	tr      *netease.Track
	scroll  ui.ScrollState
	exclude int64 // the playlist already showing the song
}

// collectButton is the + of the playing page's bar.
func (a *app) collectButton(c *ui.Context, tr *netease.Track) {
	b := iconButton(c.Key("collect"), iconCollect, 17, white.Alpha(0.7), "收藏到歌单")
	if b.Clicked() {
		a.openCollect(tr)
	}
}

// openCollect asks which playlist tr goes into. Signed out, it asks the
// listener to sign in, as liking does.
func (a *app) openCollect(tr *netease.Track) {
	if a.profile == nil {
		a.showMain()
		a.openLogin()
		return
	}
	a.collecting = collectDialog{open: true, tr: tr}
}

// collectInto puts tr in pl: in the liked songs, which is a like, or in a
// playlist of the listener's. A toast says how it went.
func (a *app) collectInto(pl *netease.Playlist, tr *netease.Track) {
	a.collecting.open = false
	if pl.SpecialType == 5 {
		// The heart under the song does the liking, and can undo it: here
		// the liked songs only take a song they don't hold.
		if a.isLiked(tr) {
			a.toast = "歌曲已在歌单中"
			return
		}
		a.toggleLike(tr)
		a.toast = "已添加到我喜欢的音乐"
		return
	}
	a.addTo(pl.ID, tr)
}

// collectDialogBody shows the playlists the chosen song may be added to.
func (a *app) collectDialogBody(c *ui.Context) {
	d := &a.collecting
	if !d.open || d.tr == nil {
		return
	}
	_, wh := c.Size()
	p := a.collectPalette()
	ui.DialogBase(c.Key("collect"), &d.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.42))
		panel.Width(400).Height(min(480, wh-140)).Padding(0).Radius(radiusPanel).
			Background(p.bg).Border(1, p.hairline).
			Shadow(0, 30, 70, -20, ui.RGBA(0, 0, 0, 0.55)).
			Transition(ui.ElementTransition{Animation: &ui.Snappy, Enter: &ui.Motion{Scale: 0.94}, Exit: &ui.Motion{Scale: 0.96}})
		panel.Children(func() {
			// Its head: what it is, and its closing, the cross a square
			// beside the title that the title centers on.
			ui.Row(c).Height(56).Padding(0, 14).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Width(31)
				ui.Text(c, "收藏到歌单").FontSize(15).FontWeight(700).Grow(1).TextAlign(ui.Center).TextColor(p.text).SingleLine()
				if iconButton(c.Key("close-collect"), iconX, 15, p.muted, "关闭").Clicked() {
					d.open = false
				}
			})
			// The playlists, the ones made first, as NetEase lists them.
			ui.Scroll(c.Key("collect-list")).Grow(1).TrackScroll(&d.scroll).Padding(4, 8, 14, 8).Gap(2).Children(func() {
				a.collectCreateRow(c, d)
				for i := range a.mine {
					if a.mine[i].ID != d.exclude {
						a.collectPlaylistRow(c, &a.mine[i], d)
					}
				}
			})
		})
	})
}

// collectRow is the look of a row of the playlists: the tile at its left
// and its words.
func collectRow(c *ui.Context, p palette, key any, label string) ui.Element {
	row := ui.ButtonBase(c.Key(key)).Height(56).Padding(4, 6).Radius(radiusRow).Gap(12).AlignItems(ui.Center).Justify(ui.Start).FocusRing(false).Label(label)
	rg := ui.Transparent
	if row.Hovered() {
		rg = p.hover
	}
	row.Background(row.AnimateColor("bg", rg, hoverFade))
	return row
}

// collectCreateRow is the first row: it asks for the name of a playlist to
// make, the song put in it once it is.
func (a *app) collectCreateRow(c *ui.Context, d *collectDialog) {
	p := a.collectPalette()
	row := collectRow(c, p, "create", "创建新歌单")
	row.Children(func() {
		ui.Box(c).Size(40, 40).Radius(radiusTile).Center().Background(p.hover).Children(func() {
			ui.Icon(c, iconPlus).Size(16, 16).TextColor(p.muted)
		})
		ui.Text(c, "创建新歌单").FontSize(13.5).FontWeight(600).TextColor(p.text)
	})
	if row.Clicked() {
		d.open = false
		a.newPlaylist(d.tr)
	}
}

// collectPlaylistRow is a playlist the song may go into: its cover, its
// name and how many songs it holds. The liked songs carry a heart on
// theirs.
func (a *app) collectPlaylistRow(c *ui.Context, pl *netease.Playlist, d *collectDialog) {
	p := a.collectPalette()
	row := collectRow(c, p, pl.ID, "添加到歌单："+pl.Name)
	row.Children(func() {
		cov := ui.Box(c).Size(40, 40).Radius(radiusTile).Background(p.hover)
		cov.Children(func() { a.coverFill(c, pl.Cover(), 120, radiusTile) })
		if pl.SpecialType == 5 {
			cov.Background(p.accent.Alpha(0.14))
			cov.Children(func() {
				ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Center().Children(func() {
					ui.Icon(c, iconHeartFill).Size(14, 14).TextColor(p.accent)
				})
			})
		}
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, pl.Name).FontSize(13.5).FontWeight(600).TextColor(p.text).SingleLine()
			if n := pl.TrackCount; n > 0 {
				ui.Text(c, count(n)+"首").FontSize(11.5).TextColor(p.muted).SingleLine()
			}
		})
	})
	if row.Clicked() {
		a.collectInto(pl, d.tr)
	}
}

// collectPalette keeps the playing page's panel dark and follows the other pages.
func (a *app) collectPalette() palette {
	if a.npOpen {
		return paletteFor(&ui.Theme{Dark: true})
	}
	return a.pal
}
