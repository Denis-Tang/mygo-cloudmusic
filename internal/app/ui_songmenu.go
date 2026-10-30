package app

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// copyText puts text on the clipboard; tests replace it.
var copyText = func(text string) error { return mygo.Clipboard.Write(transfer.TextData(text)) }

type songMenuState struct {
	open, pointer bool
	x, y          float32
}

// songMenu gives a row a visible action button and the same menu on right click.
func (a *app) songMenu(c *ui.Context, row ui.Element, tr *netease.Track, pid int64, color ui.Color) {
	st := ui.Local(row, "song-menu", func() songMenuState { return songMenuState{} })
	if row.RightClicked() {
		st.open, st.pointer = true, true
		st.x, st.y, _ = row.PointerPosition()
	}
	b := iconButton(c.Key("song-more"), iconMore, 16, color, "歌曲操作："+tr.Name).Size(28, 28).Expanded(st.open)
	if b.Clicked() {
		st.open, st.pointer = !st.open, false
	}
	row.OnShortcut(ui.Shift, ui.KeyF10, func() { st.open, st.pointer = true, false })
	anchor := b
	if st.pointer && st.open {
		anchor = ui.Box(c.Key("song-pointer")).Absolute().Left(st.x).Top(st.y).Size(0, 0)
	}
	a.actionPopover(c.Key("song-actions"), anchor, &st.open, func() {
		first := true
		item := func(icon *ui.SVG, label string, action func()) {
			b := a.menuItem(c, icon, label, "")
			if first {
				b.AutoFocus()
				first = false
			}
			b.OnClick(func() { st.open = false; action() })
		}
		if tr.ProgramID != 0 {
			item(iconCopy, "复制节目名", func() { copyText(tr.Name) })
			item(iconLink, "复制链接", func() { copyText(tr.URL()) })
			item(iconPodcast, "查看播客", func() { a.openPage(route{page: pagePodcast, id: tr.PodcastID}) })
			return
		}
		item(iconCollect, "添加到歌单", func() { a.openCollect(tr); a.collecting.exclude = pid })
		for _, ar := range tr.Artists {
			if ar.ID != 0 {
				item(iconUser, "查看歌手："+ar.Name, func() { a.openArtist(ar.ID) })
			}
		}
		if tr.Album.ID != 0 {
			item(iconCover, "查看专辑："+tr.Album.Name, func() { a.openPage(route{page: pageAlbum, id: tr.Album.ID}) })
		}
		ui.Box(c).Height(1).Margin(4, 8).Background(a.pal.hairline)
		item(iconCopy, "复制歌名", func() { copyText(tr.Name) })
		item(iconCopy, "复制歌名和歌手", func() { copyText(tr.Name + " - " + tr.ArtistNames()) })
		item(iconLink, "复制链接", func() { copyText(tr.URL()) })
		if a.editable(pid) != nil {
			ui.Box(c).Height(1).Margin(4, 8).Background(a.pal.hairline)
			item(iconTrash, "从歌单中移除", func() { a.removeFrom(pid, tr) })
		}
	})
}

// actionPopover uses the account menu's material and keyboard navigation.
func (a *app) actionPopover(c *ui.Context, anchor ui.Element, open *bool, items func()) {
	p := a.pal
	if a.npOpen {
		p = paletteFor(&ui.Theme{Dark: true})
	}
	pop := popoverColors(p.dark)
	_, h := c.Size()
	ui.PopoverBase(c, anchor, open, func(panel ui.Element) {
		panel.Width(320).Padding(5).Radius(radiusCard).Background(pop.fill).Border(0.5, pop.edge).
			Shadow(0, 8, 24, -4, ui.RGBA(0, 0, 0, 0.2)).TextColor(p.text).
			Transition(ui.ElementTransition{Animation: &ui.Snappy, Enter: &ui.Motion{Scale: 0.96, Y: -4}, Exit: &ui.Motion{Scale: 0.98}})
		ui.Scroll(c).MaxHeight(min(520, h-48)).Role(ui.RoleMenu).Label("操作菜单").FocusGroup(ui.Vertical).Children(items)
	})
}

// playlistMenu shares the rounded action panel between a playlist and its header.
func (a *app) playlistMenu(c *ui.Context, anchor ui.Element, pl *netease.Playlist, button bool) {
	open := ui.Local(anchor, "playlist-menu", func() bool { return false })
	if anchor.RightClicked() || button && anchor.Clicked() {
		*open = !*open
	}
	anchor.OnShortcut(ui.Shift, ui.KeyF10, func() { *open = true })
	a.actionPopover(c.Key("playlist-actions"), anchor, open, func() {
		a.menuItem(c, iconPencil, "重命名…", "").AutoFocus().OnClick(func() { *open = false; a.startRename(pl) })
		ui.Box(c).Height(1).Margin(4, 8).Background(a.pal.hairline)
		a.menuItem(c, iconTrash, "删除歌单…", "").OnClick(func() { *open = false; a.confirmDelete(pl) })
	})
}
