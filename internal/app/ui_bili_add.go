package app

import (
	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// A playlistAdding is a song of this machine being put into a playlist of
// this machine's, which the dialog lists.
type playlistAdding struct {
	open  bool
	track netease.Track
}

// addToBiliPlaylist puts a song kept here into one of the playlists kept
// here, once.
func (a *app) addToBiliPlaylist(id int64, tr netease.Track) {
	pl := a.biliPlaylist(id)
	if pl == nil {
		return
	}
	for i := range pl.Items {
		if pl.Items[i].Track.ID == tr.ID {
			a.toast = "已经在这个歌单里"
			a.redraw()
			return
		}
	}
	pl.Items = append(pl.Items, biliItem{Track: tr})
	a.saveBiliPlaylists()
	a.toast = "已添加到「" + pl.Name + "」"
	a.plAdding.open = false
	a.redraw()
}

// addToPlaylistDialog lists the playlists kept here a song may go into.
func (a *app) addToPlaylistDialog(c *ui.Context) {
	p := a.pal
	d := &a.plAdding
	ui.DialogBase(c.Key("local-add"), &d.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.35))
		panel.Width(400).MaxHeight(520).Radius(radiusPanel).Clip().
			Background(p.bg).Border(1, p.hairline).Label("添加到歌单").
			Shadow(0, 24, 60, -12, ui.RGBA(0, 0, 0, 0.4)).
			Transition(ui.ElementTransition{Animation: &ui.Snappy, Enter: &ui.Motion{Scale: 0.94}, Exit: &ui.Motion{Scale: 0.96}})
		panel.Children(func() {
			ui.Row(c).Height(56).PaddingX(24).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "添加到歌单").FontSize(20).FontWeight(700).Grow(1)
				if iconButton(c, iconX, 16, p.muted, "关闭添加到歌单").Clicked() {
					d.open = false
				}
			})
			ui.Text(c, d.track.Name).FontSize(14).TextColor(p.text).Padding(0, 24, 8, 24).MaxLines(1)
			ui.Scroll(c).Grow(1).MinHeight(0).Padding(4, 16, 16, 16).Gap(2).Children(func() {
				row := func(key any, cover, name string, add func()) {
					b := ui.ButtonBase(c.Key(key)).Height(48).PaddingX(8).Gap(10).Radius(radiusRow).
						AlignItems(ui.Center).Justify(ui.Start).Label("添加到 " + name)
					if b.Hovered() {
						b.Background(p.hover)
					}
					b.Children(func() {
						a.cover(c, cover, 80, 26, radiusTile-1)
						ui.Text(c, name).FontSize(14).Grow(1).Shrink(1).MinWidth(0).SingleLine()
					})
					if b.Clicked() {
						add()
					}
				}
				site := append(append([]netease.Playlist(nil), a.mine...), a.saved...)
				if len(site) > 0 {
					ui.Text(c, "网易云歌单").FontSize(12).FontWeight(600).TextColor(p.muted).Padding(6, 8, 2, 8)
					for i := range site {
						pl := site[i]
						row(pl.ID, pl.Cover(), pl.Name, func() { a.addLocalToPlaylist(pl.ID, d.track) })
					}
				}
				kept := a.biliKept()
				if len(kept) > 0 {
					ui.Text(c, "本机歌单").FontSize(12).FontWeight(600).TextColor(p.muted).Padding(12, 8, 2, 8)
					for _, pl := range kept {
						pl := pl
						row(pl.ID, pl.Cover, pl.Name, func() { a.addToBiliPlaylist(pl.ID, d.track) })
					}
				}
				if len(site) == 0 && len(kept) == 0 {
					ui.Text(c, "还没有歌单").FontSize(14).TextColor(p.muted).Padding(10, 8, 24, 8)
				}
			})
		})
	})
}
