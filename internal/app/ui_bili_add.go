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
		panel.Width(360).MaxHeight(520).Radius(radiusPanel).Clip().
			Background(p.bg).Border(1, p.hairline).Label("添加到歌单").
			Shadow(0, 24, 60, -12, ui.RGBA(0, 0, 0, 0.4)).
			Transition(ui.ElementTransition{Animation: &ui.Snappy, Enter: &ui.Motion{Scale: 0.94}, Exit: &ui.Motion{Scale: 0.96}})
		panel.Children(func() {
			ui.Row(c).Height(52).PaddingX(20).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "添加到歌单").FontSize(17).FontWeight(700).Grow(1)
				if iconButton(c, iconX, 16, p.muted, "关闭添加到歌单").Clicked() {
					d.open = false
				}
			})
			ui.Text(c, d.track.Name).FontSize(13).TextColor(p.muted).Padding(0, 20, 8, 20).MaxLines(1)
			kept := a.biliKept()
			if len(kept) == 0 {
				ui.Text(c, "还没有本机歌单").FontSize(13).TextColor(p.muted).Padding(10, 20, 20, 20)
				return
			}
			ui.Scroll(c).Grow(1).MinHeight(0).Padding(4, 12, 16, 12).Gap(2).Children(func() {
				for _, pl := range kept {
					pl := pl
					b := ui.ButtonBase(c.Key(pl.ID)).Height(40).PaddingX(8).Gap(10).Radius(radiusRow).
						AlignItems(ui.Center).Justify(ui.Start).Label("添加到 " + pl.Name)
					if b.Hovered() {
						b.Background(p.hover)
					}
					b.Children(func() {
						a.cover(c, pl.Cover, 80, 26, radiusTile-1)
						ui.Text(c, pl.Name).FontSize(14).Grow(1).Shrink(1).MinWidth(0).SingleLine()
					})
					if b.Clicked() {
						a.addToBiliPlaylist(pl.ID, d.track)
					}
				}
			})
		})
	})
}
