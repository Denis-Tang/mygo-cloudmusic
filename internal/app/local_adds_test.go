package app

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/bilibili"
	"netease-music-mygo/internal/netease"
)

// localAddsFile puts the store in the test's folder, and restores it after.
func localAddsFileAt(t *testing.T) {
	t.Helper()
	old := localAddsFile
	dir := t.TempDir()
	localAddsFile = func() (string, error) { return filepath.Join(dir, "local_adds.json"), nil }
	t.Cleanup(func() { localAddsFile = old })
}

// A song of this machine put into a site playlist shows after its own, and
// goes away from this machine alone.
func TestLocalAddShowsAfterThePlaylistsOwn(t *testing.T) {
	localAddsFileAt(t)
	a := newTestApp()
	a.settings = defaultSettings()
	pl := &netease.Playlist{ID: 7, Name: "睡前", Tracks: []netease.Track{{ID: 1, Name: "云端一"}, {ID: 2, Name: "云端二"}}}
	a.playlists[7] = pl
	local := netease.Track{ID: -9, Name: "切片歌"}
	a.addLocalToPlaylist(7, local)
	if len(pl.Tracks) != 3 || pl.Tracks[2].Name != "切片歌" {
		t.Fatalf("tracks %+v, want the local song after the site's", pl.Tracks)
	}
	a.addLocalToPlaylist(7, local)
	if len(pl.Tracks) != 3 {
		t.Fatalf("tracks %+v, want it added once", pl.Tracks)
	}
	// Kept for the next start.
	b := newTestApp()
	b.localAdds = map[int64][]biliItem{}
	b.loadLocalAdds()
	if len(b.localAdds[7]) != 1 || b.localAdds[7][0].Track.Name != "切片歌" {
		t.Fatalf("loaded %+v", b.localAdds)
	}
	reloaded := &netease.Playlist{ID: 7, Tracks: []netease.Track{{ID: 1}, {ID: 2}}}
	b.mergeLocalAdds(reloaded)
	if len(reloaded.Tracks) != 3 || reloaded.Tracks[2].ID != -9 {
		t.Fatalf("reloaded %+v, want the local song back after a refresh", reloaded.Tracks)
	}
	if !b.removeLocalFromPlaylist(7, -9) || len(b.localAdds[7]) != 0 {
		t.Fatalf("removed %v, adds %+v", false, b.localAdds)
	}
}

// A playlist asked for by a BV number opens the one it made before, rather
// than making it again.
func TestBiliNewOpensThePlaylistItMade(t *testing.T) {
	playlistFile(t)
	a := newTestApp()
	a.settings = defaultSettings()
	v := bilibili.Video{Bvid: "BV1xx411c7mD", Aid: 42, Title: "一首歌", Duration: 180000}
	pl := a.makeBiliPlaylist(v, nil, false)
	pl.Collected = true
	a.biliPlaylists = []biliPlaylist{pl}
	a.biliNewPL = biliNewPlaylist{open: true, input: v.Bvid}
	a.biliNewResolved(v)
	if a.route.page != pageBiliPlaylist || a.route.id != pl.ID {
		t.Fatalf("route %+v, want the playlist it made", a.route)
	}
	if a.biliNewPL.open {
		t.Fatal("the dialog stayed open")
	}
	if a.biliDraft.open {
		t.Fatal("a second playlist was started")
	}
}

// A new one reads the video's songs first, and asks to keep them.
func TestBiliNewStartsReadingTheVideo(t *testing.T) {
	playlistFile(t)
	a := newTestApp()
	a.settings = defaultSettings()
	v := bilibili.Video{Bvid: "BV1xx411c7mD", Aid: 42, Title: "一首歌", Duration: 180000}
	a.biliNewPL = biliNewPlaylist{open: true, input: v.Bvid}
	a.biliNewResolved(v)
	if !a.biliDraft.open || a.biliDraft.video == nil || a.biliDraft.video.Bvid != v.Bvid {
		t.Fatalf("draft %+v, want the video read", a.biliDraft)
	}
	if len(a.biliPlaylists) != 0 {
		t.Fatalf("playlists %+v, want nothing kept before the listener says so", a.biliPlaylists)
	}
}

// Taking a song added here out of a site playlist stays on this machine:
// the site is never asked, as it never had the song.
func TestRemovingAnAddedSongLeavesTheSiteAlone(t *testing.T) {
	localAddsFileAt(t)
	asked := false
	old := removeFromPlaylist
	removeFromPlaylist = func(int64, ...int64) error { asked = true; return nil }
	t.Cleanup(func() { removeFromPlaylist = old })
	a := newTestApp()
	a.settings = defaultSettings()
	pl := &netease.Playlist{ID: 7, Tracks: []netease.Track{}}
	a.playlists[7] = pl
	tr := netease.Track{ID: -9, Name: "切片歌"}
	a.addLocalToPlaylist(7, tr)
	if len(pl.Tracks) != 1 {
		t.Fatalf("tracks %+v, want the added song", pl.Tracks)
	}
	a.removeFrom(7, &tr)
	if asked {
		t.Fatal("the site was asked to remove a song it never had")
	}
	if len(a.localAdds[7]) != 0 || len(pl.Tracks) != 0 {
		t.Fatalf("adds %+v, tracks %+v, want both empty", a.localAdds[7], pl.Tracks)
	}
	if a.toast != "已从歌单移除" {
		t.Fatalf("toast %q", a.toast)
	}
}

// A video's row in a Bilibili playlist offers itself to a playlist, and a
// site playlist takes it after its own songs.
func TestVideoRowGoesIntoASitePlaylist(t *testing.T) {
	playlistFile(t)
	localAddsFileAt(t)
	a := newTestApp()
	a.settings = defaultSettings()
	v := bilibili.Video{Bvid: "BV1xx411c7mD", Aid: 42, Title: "一首歌", Up: "UP主", Duration: 180000}
	bili := a.makeBiliPlaylist(v, nil, false)
	bili.Collected = true
	a.biliPlaylists = []biliPlaylist{bili}
	site := netease.Playlist{ID: 7, Name: "睡前", Tracks: []netease.Track{{ID: 1, Name: "云端一"}}}
	a.mine = []netease.Playlist{site}
	a.playlists[7] = &netease.Playlist{ID: 7, Name: "睡前", Tracks: []netease.Track{{ID: 1, Name: "云端一"}}}
	a.show(route{page: pageBiliPlaylist, id: bili.ID})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("歌曲操作：一首歌"); err != nil {
		t.Fatalf("no menu for the video: %v", err)
	}
	tt.Frame()
	if _, ok := tt.Find("添加到歌单"); !ok {
		t.Fatal("a video's row offers no way into a playlist")
	}
	if err := tt.Click("添加到歌单"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("添加到 睡前"); err != nil {
		t.Fatalf("the site playlist is not offered: %v", err)
	}
	tt.Frame()
	got := a.playlists[7].Tracks
	if len(got) != 2 || got[1].Name != "一首歌" {
		t.Fatalf("tracks %+v, want the video after the site's song", got)
	}
	if len(a.localAdds[7]) != 1 || a.localAdds[7][0].Video == nil {
		t.Fatalf("adds %+v, want the video kept with what plays it", a.localAdds[7])
	}
}
