package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/bilibili"
	"netease-music-mygo/internal/diag"
	"netease-music-mygo/internal/netease"
)

// A biliPlaylist is a playlist kept on this machine: the videos, or later
// the songs, a video's page was turned into.
type biliPlaylist struct {
	ID      int64      `json:"id"`
	Name    string     `json:"name"`
	Origin  string     `json:"origin,omitempty"` // the video's page
	Mode    string     `json:"mode"`             // tracks, songs or slices
	Created int64      `json:"created"`
	Items   []biliItem `json:"items"`
}

// A biliItem is one row of such a playlist: a track, and the video it
// came from, if any.
type biliItem struct {
	Track netease.Track     `json:"track"`
	Video *bilibili.Video   `json:"video,omitempty"`
	Start int64             `json:"startMs,omitempty"` // for a slice
}

// biliPlaylistsFile is where the playlists are kept.
var biliPlaylistsFile = func() (string, error) {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "bili_playlists.json"), nil
}

// loadBiliPlaylists reads the playlists, and registers how to play what
// they hold.
func (a *app) loadBiliPlaylists() {
	path, err := biliPlaylistsFile()
	if err != nil {
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var list []biliPlaylist
	if json.Unmarshal(b, &list) != nil {
		return
	}
	for i := range list {
		for j := range list[i].Items {
			if v := list[i].Items[j].Video; v != nil {
				// The same ID, with the way to play it, kept afresh.
				list[i].Items[j].Track = a.biliTrack(*v)
			}
		}
	}
	a.biliPlaylists = list
}

// saveBiliPlaylists keeps the playlists.
func (a *app) saveBiliPlaylists() {
	path, err := biliPlaylistsFile()
	if err != nil {
		diag.Errorf("bili playlists: %v", err)
		return
	}
	b, err := json.MarshalIndent(a.biliPlaylists, "", "  ")
	if err != nil {
		diag.Errorf("bili playlists: %v", err)
		return
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		diag.Errorf("bili playlists: %v", err)
	}
}

// biliPlaylist finds a playlist by ID.
func (a *app) biliPlaylist(id int64) *biliPlaylist {
	for i := range a.biliPlaylists {
		if a.biliPlaylists[i].ID == id {
			return &a.biliPlaylists[i]
		}
	}
	return nil
}

// addBiliPlaylist makes a playlist of one video, with the name given.
func (a *app) addBiliPlaylist(name string, v *bilibili.Video) *biliPlaylist {
	pl := biliPlaylist{
		ID:      time.Now().UnixNano(),
		Name:    strings.TrimSpace(name),
		Mode:    "tracks",
		Created: time.Now().Unix(),
	}
	if pl.Name == "" {
		pl.Name = "未命名歌单"
	}
	if v != nil {
		pl.Origin = v.URL()
		pl.Items = append(pl.Items, biliItem{Track: a.biliTrack(*v), Video: v})
	}
	a.biliPlaylists = append([]biliPlaylist{pl}, a.biliPlaylists...)
	a.saveBiliPlaylists()
	diag.Logf("bili playlist %q with %d", pl.Name, len(pl.Items))
	return &a.biliPlaylists[0]
}

// deleteBiliPlaylist drops one, which cannot be undone.
func (a *app) deleteBiliPlaylist(id int64) {
	kept := a.biliPlaylists[:0]
	for _, pl := range a.biliPlaylists {
		if pl.ID != id {
			kept = append(kept, pl)
		}
	}
	a.biliPlaylists = kept
	a.saveBiliPlaylists()
	if a.route.page == pageBiliPlaylist && a.route.id == id {
		a.back()
	}
}

// renameBiliPlaylist renames one.
func (a *app) renameBiliPlaylist(id int64, name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	if pl := a.biliPlaylist(id); pl != nil {
		pl.Name = name
		a.saveBiliPlaylists()
	}
}

// openBiliNaming asks for the name of a playlist: a new one of video v, or
// the one being renamed when v is nil.
func (a *app) openBiliNaming(v *bilibili.Video, id int64, name string) {
	a.biliNaming = biliNaming{open: true, name: name, video: v, playlist: id}
}

// submitBiliNaming makes or renames the playlist the dialog asked about.
func (a *app) submitBiliNaming() {
	d := &a.biliNaming
	name := strings.TrimSpace(d.name)
	if name == "" || d.busy {
		return
	}
	if d.playlist != 0 {
		a.renameBiliPlaylist(d.playlist, name)
		d.open = false
		a.redraw()
		return
	}
	pl := a.addBiliPlaylist(name, d.video)
	d.open = false
	a.redraw()
	if pl != nil {
		a.open(route{page: pageBiliPlaylist, id: pl.ID})
	}
}

// biliPlaylistPage lists a playlist kept on this machine.
func (a *app) biliPlaylistPage(c *ui.Context) {
	pl := a.biliPlaylist(a.route.id)
	if pl == nil {
		a.centered(c, "这个歌单不在了", nil)
		return
	}
	tracks := make([]netease.Track, len(pl.Items))
	for i := range pl.Items {
		tracks[i] = pl.Items[i].Track
		if v := pl.Items[i].Video; v != nil {
			// The page says whether the site has these songs, so it looks.
			a.probeBiliSource(tracks[i].ID, *v)
		}
	}
	a.trackRows(c, tracks, true, func(c *ui.Context) { a.biliPlaylistHeader(c, pl) })
}

// biliPlaylistHeader is the page's head: the cover, the name, what it
// holds, and what may be done with it.
func (a *app) biliPlaylistHeader(c *ui.Context, pl *biliPlaylist) {
	p := a.pal
	cover := ""
	if len(pl.Items) > 0 {
		cover = pl.Items[0].Track.Album.PicURL
	}
	ui.Row(c).Gap(28).AlignItems(ui.End).Padding(0, 0, 26, 0).Children(func() {
		ui.Box(c).Size(212, 212).Radius(radiusCard).Shadow(0, 18, 40, -10, p.cardShade).Children(func() {
			a.coverFill(c, cover, 600, radiusCard)
		})
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
			ui.Text(c, "本机歌单 · 哔哩哔哩").FontSize(headSub).FontWeight(600).TextColor(p.accent)
			ui.Text(c, pl.Name).FontSize(headTitle).FontWeight(800).LetterSpacing(-0.6).MaxLines(2).LineHeight(1.2)
			ui.Text(c, fmt.Sprintf("%d 首", len(pl.Items))).FontSize(15).TextColor(p.muted)
			ui.Row(c).Gap(8).Margin(6, 0, 0, 0).Children(func() {
				if pl.Origin != "" {
					if pillButton(c.Key("bili-origin"), iconLink, "打开视频页", p.hover.Over(p.bg), p.text).Clicked() {
						mygo.Shell.OpenExternal(pl.Origin)
					}
				}
				if pillButton(c.Key("bili-rename"), iconPencil, "重命名", p.hover.Over(p.bg), p.text).Clicked() {
					a.openBiliNaming(nil, pl.ID, pl.Name)
				}
				if pillButton(c.Key("bili-delete"), iconTrash, "删除", p.hover.Over(p.bg), p.text).Clicked() {
					a.biliDeleting = biliDeleting{open: true, playlist: pl.ID, name: pl.Name}
				}
			})
		})
	})
}

// biliNaming is the dialog that asks a playlist's name.
type biliNaming struct {
	open     bool
	name     string
	playlist int64
	video    *bilibili.Video
	busy     bool
}

// biliDeleting is the dialog that asks whether to delete a playlist.
type biliDeleting struct {
	open     bool
	playlist int64
	name     string
}

// biliNameDialog asks for a playlist's name.
func (a *app) biliNameDialog(c *ui.Context) {
	p := a.pal
	d := &a.biliNaming
	ui.DialogBase(c.Key("bili-playlist-name"), &d.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.35))
		panel.Width(380).Padding(24).Radius(radiusPanel).Gap(16).
			Background(p.bg).Border(1, p.hairline).Shadow(0, 24, 60, -12, ui.RGBA(0, 0, 0, 0.4)).
			Transition(ui.ElementTransition{Animation: &ui.Snappy, Enter: &ui.Motion{Scale: 0.94}, Exit: &ui.Motion{Scale: 0.96}})
		title, action := "收藏为歌单", "创建"
		if d.playlist != 0 {
			title, action = "重命名歌单", "保存"
		}
		ui.Text(c, title).FontSize(17).FontWeight(700)
		if d.video != nil {
			ui.Text(c, "把《"+d.video.Title+"》存成一个本机歌单。").FontSize(14).TextColor(p.muted).LineHeight(1.45)
		}
		in := ui.TextInput(c.Key("bili-name"), &d.name).Placeholder("歌单名称").Label("歌单名称").FontSize(17).Radius(radiusField).AutoFocus()
		if in.Submitted() {
			a.submitBiliNaming()
		}
		ui.Row(c).Gap(10).Justify(ui.End).Children(func() {
			if pillButton(c.Key("cancel"), nil, "取消", p.hover.Over(p.bg), p.text).Clicked() {
				d.open = false
			}
			fill, ink := p.accent, p.onAccent
			ok := pillButton(c.Key("ok"), nil, action, fill, ink)
			if strings.TrimSpace(d.name) == "" {
				ok.Disabled(true)
			}
			if ok.Clicked() {
				a.submitBiliNaming()
			}
		})
	})
}

// biliDeleteDialog asks whether to delete a playlist, which cannot be
// undone.
func (a *app) biliDeleteDialog(c *ui.Context) {
	p := a.pal
	d := &a.biliDeleting
	ui.DialogBase(c.Key("bili-playlist-delete"), &d.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.35))
		panel.Width(360).Padding(24).Radius(radiusPanel).Gap(8).
			Background(p.bg).Border(1, p.hairline).Shadow(0, 24, 60, -12, ui.RGBA(0, 0, 0, 0.4)).
			Transition(ui.ElementTransition{Animation: &ui.Snappy, Enter: &ui.Motion{Scale: 0.94}, Exit: &ui.Motion{Scale: 0.96}})
		ui.Text(c, "删除歌单").FontSize(17).FontWeight(700)
		ui.Text(c, "确定删除歌单“"+d.name+"”吗？删除后无法恢复。").FontSize(16).TextColor(p.muted).LineHeight(1.45)
		ui.Row(c).Gap(10).Justify(ui.End).Margin(8, 0, 0, 0).Children(func() {
			if pillButton(c.Key("cancel"), nil, "取消", p.hover.Over(p.bg), p.text).Clicked() {
				d.open = false
			}
			if pillButton(c.Key("ok"), nil, "删除", p.accent, p.onAccent).Clicked() {
				a.deleteBiliPlaylist(d.playlist)
				d.open = false
			}
		})
	})
}

// biliPlaylistsSidebar lists the playlists kept on this machine, under a
// header of their own: apart from the ones of NetEase's.
func (a *app) biliPlaylistsSidebar(c *ui.Context) {
	if len(a.biliPlaylists) == 0 {
		return
	}
	p := a.pal
	title := "哔哩哔哩歌单"
	ui.Row(c.Key(title)).AlignItems(ui.Center).Margin(18, 0, 6, 6).Children(func() {
		b := ui.ButtonBase(c.Key("toggle")).Grow(1).Height(32).PaddingX(5).Gap(7).Radius(radiusTile).
			AlignItems(ui.Center).Justify(ui.Start).Role(ui.RoleDisclosure).Label(title)
		if b.Clicked() {
			a.biliPLCollapsed = !a.biliPLCollapsed
		}
		b.Expanded(!a.biliPLCollapsed)
		hb := ui.Transparent
		if b.Hovered() {
			hb = p.hover
		}
		b.Background(b.AnimateColor("bg", hb, ui.Animation{Duration: 120 * time.Millisecond}))
		b.Children(func() {
			ui.Text(c, title).FontSize(sideFont(11.5)).FontWeight(600).TextColor(p.faint).Grow(1)
			arrow := iconDown
			if a.biliPLCollapsed {
				arrow = iconForward
			}
			ui.Icon(c, arrow).Size(15, 15).TextColor(p.faint)
		})
	})
	if a.biliPLCollapsed {
		return
	}
	for i := range a.biliPlaylists {
		pl := &a.biliPlaylists[i]
		selected := a.route.page == pageBiliPlaylist && a.route.id == pl.ID && !a.nowPlaying
		b := ui.ButtonBase(c.Key(pl.ID)).Width(fullSidebar - 38).Height(42).PaddingX(9).Gap(11).Radius(radiusRow).AlignItems(ui.Center).Justify(ui.Start).FocusRing(false).Label(pl.Name)
		pb := ui.Transparent
		switch {
		case selected:
			pb = p.press
		case b.Hovered():
			pb = p.hover
		}
		b.Background(b.AnimateColor("bg", pb, ui.Animation{Duration: 120 * time.Millisecond}))
		cover := ""
		if len(pl.Items) > 0 {
			cover = pl.Items[0].Track.Album.PicURL
		}
		b.Children(func() {
			a.cover(c, cover, 80, 28, radiusTile-1)
			ui.Text(c, pl.Name).FontSize(sideFont(13)).Grow(1).Shrink(1).MinWidth(0).SingleLine()
		})
		if b.Clicked() {
			a.open(route{page: pageBiliPlaylist, id: pl.ID})
		}
	}
}
