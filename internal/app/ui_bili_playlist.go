package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/bilibili"
	"netease-music-mygo/internal/diag"
	"netease-music-mygo/internal/netease"
)

// biliIDSeq counts the playlists made, so that two made in a moment do not
// share an ID, which the sidebar keys its rows by.
var biliIDSeq atomic.Int64

// biliNewID is a new playlist's ID, later than the one before it.
func biliNewID() int64 {
	for {
		last := biliIDSeq.Load()
		id := max(time.Now().UnixNano(), last+1)
		if biliIDSeq.CompareAndSwap(last, id) {
			return id
		}
	}
}

// A biliPlaylist is a playlist kept on this machine: the videos, or later
// the songs, a video's page was turned into.
type biliPlaylist struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Origin  string `json:"origin,omitempty"` // the video's page
	Cover   string `json:"cover,omitempty"`  // the video's cover
	Mode    string `json:"mode"`             // tracks, songs or slices
	Created int64  `json:"created"`
	Cloud   int64  `json:"cloudID,omitempty"` // the site's playlist it went to
	// Collected says the listener keeps it among what they collect, which
	// stays on this machine: the site only holds the songs found.
	Collected bool `json:"collected,omitempty"`
	// Remote is the Bilibili collection this playlist was imported from, so
	// that importing it again updates it rather than adds another.
	Remote int64      `json:"remoteID,omitempty"`
	Items  []biliItem `json:"items"`
}

// A biliItem is one row of such a playlist: a track, and the video it
// came from, if any.
type biliItem struct {
	Track netease.Track   `json:"track"`
	Video *bilibili.Video `json:"video,omitempty"`
	Start int64           `json:"startMs,omitempty"` // for a slice
	End   int64           `json:"endMs,omitempty"`   // where the slice ends
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
			item := &list[i].Items[j]
			if v := item.Video; v != nil {
				// The same ID, with the way to play it, kept afresh.
				if item.End > item.Start {
					item.Track = a.biliSliceTrack(*v, item.Track.Name, item.Start, item.End)
					a.bindBiliSlice(*v, item.Track.ID, item.Start, item.End)
				} else {
					item.Track = a.biliTrack(*v)
				}
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

// biliPlaylist finds a playlist by ID, kept or a preview of a video.
func (a *app) biliPlaylist(id int64) *biliPlaylist {
	for i := range a.biliPlaylists {
		if a.biliPlaylists[i].ID == id {
			return &a.biliPlaylists[i]
		}
	}
	if p := a.biliPreview; p != nil && p.ID == id {
		return p
	}
	return nil
}

// keepBiliPlaylist puts a preview among the playlists kept, where it was
// collected; a playlist kept already is returned as it is.
func (a *app) keepBiliPlaylist(pl *biliPlaylist) *biliPlaylist {
	if pl == nil || pl != a.biliPreview {
		return pl
	}
	kept := *pl
	a.biliPreview = nil
	a.biliPlaylists = append([]biliPlaylist{kept}, a.biliPlaylists...)
	a.saveBiliPlaylists()
	return &a.biliPlaylists[0]
}

// deleteBiliPlaylist drops one, which cannot be undone.
func (a *app) deleteBiliPlaylist(id int64) {
	if p := a.biliPreview; p != nil && p.ID == id {
		a.biliPreview = nil
		if a.route.page == pageBiliPlaylist && a.route.id == id {
			a.back()
		}
		return
	}
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

// openBiliRename asks for the name a playlist takes here.
func (a *app) openBiliRename(id int64, name string) {
	a.biliNaming = biliNaming{open: true, name: name, playlist: id}
}

// submitBiliNaming renames the playlist the dialog asked about.
func (a *app) submitBiliNaming() {
	d := &a.biliNaming
	name := strings.TrimSpace(d.name)
	if name == "" || d.busy || d.playlist == 0 {
		return
	}
	a.renameBiliPlaylist(d.playlist, name)
	d.open = false
	a.redraw()
}

// biliPlaylistPage lists a playlist kept on this machine, or a preview of
// a video not collected yet.
func (a *app) biliPlaylistPage(c *ui.Context) {
	pl := a.biliPlaylist(a.route.id)
	if pl == nil {
		a.centered(c, "这个歌单不在了", nil)
		return
	}
	tracks := make([]netease.Track, len(pl.Items))
	// A collection imported from an account may hold hundreds of videos:
	// the site is not asked about each of them, which would be as many
	// searches as it has rows. A playlist made from one video asks, as the
	// page says what the site has for its songs.
	look := pl.Remote == 0 && pl.Mode != "slices"
	for i := range pl.Items {
		tracks[i] = pl.Items[i].Track
		if v := pl.Items[i].Video; v != nil && look {
			a.probeBiliSource(tracks[i].ID, *v)
		}
	}
	a.trackRows(c, tracks, true, func(c *ui.Context) { a.biliPlaylistHeader(c, pl) })
}

// biliPlaylistHeader is the page's head: the video's cover, the name,
// what it holds, and what may be done with it.
func (a *app) biliPlaylistHeader(c *ui.Context, pl *biliPlaylist) {
	p := a.pal
	cover := pl.Cover
	if cover == "" && len(pl.Items) > 0 {
		// A playlist kept before covers were: the first song's stands in.
		cover = pl.Items[0].Track.Album.PicURL
	}
	tracks := biliPlayTracks(pl)
	ui.Row(c).Gap(28).AlignItems(ui.End).Padding(0, 0, 24, 0).Children(func() {
		ui.Box(c).Size(212, 212).Radius(radiusCard).Shadow(0, 18, 40, -10, p.cardShade).Children(func() {
			a.coverFill(c, cover, 1200, radiusCard)
		})
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
			kind := "本机歌单 · 哔哩哔哩"
			if pl == a.biliPreview {
				kind = "临时预览 · 哔哩哔哩"
			}
			ui.Text(c, kind).FontSize(headSub).FontWeight(600).TextColor(p.accent)
			ui.Text(c, pl.Name).FontSize(headTitle).FontWeight(800).LetterSpacing(-0.6).MaxLines(2).LineHeight(1.2)
			ui.Text(c, fmt.Sprintf("%d 首", len(pl.Items))).FontSize(15).TextColor(p.faint)
			ui.Row(c).Wrap().Gap(10).Margin(10, 0, 0, 0).Children(func() {
				if pillButton(c.Key("bili-play"), iconPlay, "播放", p.accent, p.onAccent).Clicked() {
					if a.pl.Shuffle() {
						a.pl.ToggleShuffle()
					}
					a.pl.PlayQueue(tracks, firstPlayable(tracks))
				}
				icon, label := iconCollect, "收藏"
				switch {
				case pl.Cloud != 0:
					icon, label = iconCheck, "已收藏到网易云"
				case pl.Collected:
					icon, label = iconCheck, "已收藏到本机"
				}
				if pillButton(c.Key("bili-collect"), icon, label, p.hover.Over(p.bg), p.text).Clicked() {
					a.collectBiliPlaylist(pl)
				}
				if pillButton(c.Key("bili-shuffle"), iconShuffle, "随机播放", p.hover.Over(p.bg), p.text).Clicked() {
					if !a.pl.Shuffle() {
						a.pl.ToggleShuffle()
					}
					a.pl.PlayQueue(tracks, firstPlayable(tracks))
				}
				// The rest of what may be done is under the pointer's three
				// dots, rather than a row of buttons beside the play ones.
				more := iconButton(c.Key("bili-more"), iconMore, 18, p.text, "更多操作")
				open := ui.Local(more, "bili-more-open", func() bool { return false })
				if more.Clicked() {
					*open = !*open
				}
				more.Expanded(*open)
				a.actionPopover(c.Key("bili-playlist-actions"), more, open, func() {
					close := func() { *open = false }
					// Only a playlist a video made is made again; one
					// imported from a collection is that collection.
					if v := biliPlaylistVideo(pl); v != nil && pl.Origin != "" {
						a.menuItem(c, iconReload, "重新生成", "").AutoFocus().OnClick(func() {
							close()
							a.rebuildBiliPlaylist(pl, *v)
						})
					}
					a.menuItem(c, iconPencil, "重命名…", "").OnClick(func() {
						close()
						a.openBiliRename(pl.ID, pl.Name)
					})
					ui.Box(c).Height(1).Margin(4, 8).Background(p.hairline)
					a.menuItem(c, iconTrash, "删除歌单…", "").OnClick(func() {
						close()
						a.biliDeleting = biliDeleting{open: true, playlist: pl.ID, name: pl.Name}
					})
				})
			})
		})
	})
}

// rebuildBiliPlaylist drops what the page shows and makes it again from
// the video: a preview goes without leaving anything behind.
func (a *app) rebuildBiliPlaylist(pl *biliPlaylist, v bilibili.Video) {
	if pl == a.biliPreview {
		a.biliPreview = nil
		a.openBiliVideo(v, false)
		return
	}
	a.deleteBiliPlaylist(pl.ID)
	a.openBiliVideo(v, false)
}

// biliNaming is the dialog that asks a playlist's name.
type biliNaming struct {
	open     bool
	name     string
	playlist int64
	busy     bool
}

// biliDeleting is the dialog that asks whether to delete a playlist.
type biliDeleting struct {
	open     bool
	playlist int64
	name     string
}

// biliNameDialog asks for the name a playlist takes here.
func (a *app) biliNameDialog(c *ui.Context) {
	p := a.pal
	d := &a.biliNaming
	ui.DialogBase(c.Key("bili-playlist-name"), &d.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.35))
		panel.Width(360).Padding(24).Radius(radiusPanel).Gap(16).
			Background(p.bg).Border(1, p.hairline).Shadow(0, 24, 60, -12, ui.RGBA(0, 0, 0, 0.4)).
			Transition(ui.ElementTransition{Animation: &ui.Snappy, Enter: &ui.Motion{Scale: 0.94}, Exit: &ui.Motion{Scale: 0.96}})
		ui.Text(c, "重命名歌单").FontSize(20).FontWeight(700)
		in := ui.TextInput(c.Key("bili-name"), &d.name).Placeholder("歌单名称").Label("歌单名称").FontSize(17).Radius(radiusField).AutoFocus()
		if in.Submitted() {
			a.submitBiliNaming()
		}
		ui.Row(c).Gap(10).Justify(ui.End).Children(func() {
			if pillButton(c.Key("cancel"), nil, "取消", p.hover.Over(p.bg), p.text).Clicked() {
				d.open = false
			}
			fill, ink := p.accent, p.onAccent
			ok := pillButton(c.Key("ok"), nil, "保存", fill, ink)
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
		ui.Text(c, "删除歌单").FontSize(20).FontWeight(700)
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

// biliKept are the playlists kept among what the listener collects:
// collected here or imported from the account. A playlist an older
// version kept on merely opening a video is left out, where it stays in
// the store alone.
func (a *app) biliKept() []*biliPlaylist {
	out := make([]*biliPlaylist, 0, len(a.biliPlaylists))
	for i := range a.biliPlaylists {
		pl := &a.biliPlaylists[i]
		if pl.Collected || pl.Cloud != 0 || pl.Remote != 0 {
			out = append(out, pl)
		}
	}
	return out
}

// biliPlaylistRows lists the playlists kept on this machine: the
// collections imported from an account first, then the ones collected here.
func (a *app) biliPlaylistRows(c *ui.Context, kept []*biliPlaylist) {
	var imported, collected []*biliPlaylist
	for _, pl := range kept {
		if pl.Remote != 0 {
			imported = append(imported, pl)
		} else {
			collected = append(collected, pl)
		}
	}
	a.biliPlaylistGroup(c, imported, true)
	a.biliPlaylistGroup(c, collected, false)
}

// biliPlaylistGroup lists one group of them, each a row that may be dragged
// among the group.
func (a *app) biliPlaylistGroup(c *ui.Context, lists []*biliPlaylist, imported bool) {
	group := "bili-kept"
	if imported {
		group = "bili-imported"
	}
	a.dragGroupStart()
	for i, pl := range lists {
		pl := pl
		a.biliPlaylistRow(c, pl, i, len(lists), group, func(steps int) {
			a.moveBiliPlaylist(pl.ID, steps, imported)
		})
	}
	a.dragDropsRun()
}

// biliPlaylistRow is one of them, dragging within its group when the group
// has more than one.
func (a *app) biliPlaylistRow(c *ui.Context, pl *biliPlaylist, i, count int, group string, move func(steps int)) {
	p := a.pal
	selected := a.route.page == pageBiliPlaylist && a.route.id == pl.ID && !a.nowPlaying
	b := ui.ButtonBase(c.Key(pl.ID)).Width(fullSidebar - 38).Height(42).PaddingX(12).Gap(11).Radius(radiusRow).AlignItems(ui.Center).Justify(ui.Start).FocusRing(false).Label(pl.Name)
	pb := ui.Transparent
	switch {
	case selected:
		pb = p.press
	case b.Hovered():
		pb = p.hover
	}
	b.Background(b.AnimateColor("bg", pb, ui.Animation{Duration: 120 * time.Millisecond}))
	cover := pl.Cover
	if cover == "" && len(pl.Items) > 0 {
		cover = pl.Items[0].Track.Album.PicURL
	}
	b.Children(func() {
		a.cover(c, cover, 80, 28, radiusTile-1)
		ui.Text(c, pl.Name).FontSize(sideFont(13)).Grow(1).Shrink(1).MinWidth(0).SingleLine()
	})
	if count > 1 {
		st := ui.Local(b, "drag", func() dragRow { return dragRow{} })
		a.dragRowOf(b, group, i, count, st, cover, pl.Name, move)
		a.dragShiftRow(b, group, i, st)
	}
	if b.Clicked() {
		a.open(route{page: pageBiliPlaylist, id: pl.ID})
	}
}

// biliPlaylistVideo is the video a playlist stands for, where it holds the
// video itself as its first item.
func biliPlaylistVideo(pl *biliPlaylist) *bilibili.Video {
	for i := range pl.Items {
		if v := pl.Items[i].Video; v != nil {
			return v
		}
	}
	return nil
}

// biliPlayTracks is what a playlist holds, as the player lists it.
func biliPlayTracks(pl *biliPlaylist) []netease.Track {
	tracks := make([]netease.Track, len(pl.Items))
	for i := range pl.Items {
		tracks[i] = pl.Items[i].Track
	}
	return tracks
}

// biliSongIDs are the songs of a playlist the site knows, which are the
// ones it can hold.
func biliSongIDs(pl *biliPlaylist) []int64 {
	var ids []int64
	for _, it := range pl.Items {
		if it.Track.ID > 0 {
			ids = append(ids, it.Track.ID)
		}
	}
	return ids
}

// biliCloudName is a name the site takes for a playlist: the one it is
// given, cut to the length the site allows.
func biliCloudName(name string) string {
	name = strings.TrimSpace(name)
	rs := []rune(name)
	if len(rs) > biliCloudNameMost {
		name = strings.TrimSpace(string(rs[:biliCloudNameMost]))
	}
	return strings.TrimRight(name, " -–—·|｜:：,，、")
}

// biliCloudNameMost is the longest name the site takes for a playlist.
const biliCloudNameMost = 40

// collectBiliPlaylist keeps the playlist where the settings say: here, on
// this machine, or on the site, which takes a name. A preview is kept
// here first when it goes to the site, so nothing is lost if that fails.
func (a *app) collectBiliPlaylist(pl *biliPlaylist) {
	if pl == nil {
		return
	}
	if pl.Cloud != 0 {
		a.toast = "已经收藏到网易云"
		a.redraw()
		return
	}
	if pl.Collected {
		a.toast = "已经收藏到本机"
		a.redraw()
		return
	}
	if a.settings.CollectTo == collectCloud && a.profile != nil && len(biliSongIDs(pl)) > 0 {
		a.biliCollect = biliCollecting{open: true, name: biliCloudName(pl.Name), playlist: pl.ID}
		return
	}
	switch {
	case a.settings.CollectTo != collectCloud:
		a.collectBiliHere(pl, "")
	case a.profile == nil:
		a.collectBiliHere(pl, "未登录网易云")
	default:
		a.collectBiliHere(pl, "没有可同步到网易云的歌曲")
	}
}

// collectBiliHere keeps the playlist among what the listener collects, on
// this machine alone; why says what kept it from the site, if anything.
func (a *app) collectBiliHere(pl *biliPlaylist, why string) {
	pl = a.keepBiliPlaylist(pl)
	if pl == nil {
		return
	}
	pl.Collected = true
	a.saveBiliPlaylists()
	a.toast = "已收藏到本机"
	a.prepareBiliPlaylist(pl.ID)
	if why != "" {
		a.toast += "；" + why
	}
	a.redraw()
}

// A biliCollecting is the name dialog of a playlist going to the site.
type biliCollecting struct {
	open     bool
	name     string
	playlist int64
	busy     bool
}

// biliCollectDialog asks what name the playlist takes on the site.
func (a *app) biliCollectDialog(c *ui.Context) {
	p := a.pal
	d := &a.biliCollect
	ui.DialogBase(c.Key("bili-collect"), &d.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.35))
		panel.Width(360).Padding(24).Radius(radiusPanel).Gap(14).
			Background(p.bg).Border(1, p.hairline).Shadow(0, 24, 60, -12, ui.RGBA(0, 0, 0, 0.4)).
			Transition(ui.ElementTransition{Animation: &ui.Snappy, Enter: &ui.Motion{Scale: 0.94}, Exit: &ui.Motion{Scale: 0.96}})
		ui.Text(c, "收藏到网易云").FontSize(20).FontWeight(700)
		in := ui.TextInput(c.Key("bili-collect-name"), &d.name).Placeholder("歌单名称").Label("歌单名称").FontSize(14).Radius(radiusField).AutoFocus()
		if in.Submitted() {
			a.submitBiliCollect()
		}
		ui.Row(c).Gap(10).Justify(ui.End).Children(func() {
			if pillButton(c.Key("cancel"), nil, "取消", p.hover.Over(p.bg), p.text).Clicked() {
				d.open = false
			}
			ok := pillButton(c.Key("ok"), nil, "收藏", p.accent, p.onAccent)
			if strings.TrimSpace(d.name) == "" || d.busy {
				ok.Disabled(true)
			}
			if ok.Clicked() {
				a.submitBiliCollect()
			}
		})
	})
}

// submitBiliCollect makes the playlist the dialog names on the site, and
// keeps it here among what the listener collects.
func (a *app) submitBiliCollect() {
	d := &a.biliCollect
	name := biliCloudName(d.name)
	pl := a.biliPlaylist(d.playlist)
	if name == "" || pl == nil || d.busy {
		return
	}
	ids := biliSongIDs(pl)
	if len(ids) == 0 {
		d.open = false
		a.toast = "没有可以收藏到网易云的歌曲"
		a.redraw()
		return
	}
	id := pl.ID
	d.busy = true
	a.toast = "正在收藏到网易云…"
	go func() {
		defer diag.Recover("bili-collect")
		made, err := netease.CreatePlaylist(name)
		if err == nil {
			err = netease.AddToPlaylist(made.ID, ids...)
		}
		a.post(func() {
			d.busy = false
			d.open = false
			if err != nil {
				diag.Errorf("bili collect %q: %v", name, err)
				// The site did not take it: the playlist stays here, so
				// the listener keeps what they collected.
				a.collectBiliHere(a.biliPlaylist(id), "网易云收藏失败")
				return
			}
			if kept := a.keepBiliPlaylist(a.biliPlaylist(id)); kept != nil {
				kept.Cloud = made.ID
				kept.Collected = true
				a.saveBiliPlaylists()
			}
			a.toast = "已收藏到网易云"
			a.prepareBiliPlaylist(id)
			a.redraw()
		})
	}()
}
