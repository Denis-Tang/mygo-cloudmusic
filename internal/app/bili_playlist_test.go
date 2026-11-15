package app

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/bilibili"
	"netease-music-mygo/internal/netease"
)

// playlistFile puts the store in the test's folder, and restores it after.
func playlistFile(t *testing.T) {
	t.Helper()
	old := biliPlaylistsFile
	dir := t.TempDir()
	biliPlaylistsFile = func() (string, error) { return filepath.Join(dir, "playlists.json"), nil }
	t.Cleanup(func() { biliPlaylistsFile = old })
}

// A video is kept as a playlist of this machine, apart from NetEase's,
// and is renamed or deleted there.
func TestBiliPlaylistKeepsAVideo(t *testing.T) {
	playlistFile(t)
	a := newTestApp()
	v := bilibili.Video{Bvid: "BV1xx411c7mD", Aid: 42, Title: "一首歌", Up: "UP主", Duration: 180000, PubDate: 100}
	pl := a.addBiliPlaylist("通勤", &v)
	if pl == nil || len(pl.Items) != 1 || pl.Items[0].Track.Name != "一首歌" {
		t.Fatalf("playlist %+v", pl)
	}
	if pl.Origin == "" || pl.Mode != "tracks" {
		t.Fatalf("playlist %+v, want its origin and mode", pl)
	}
	if !pl.Items[0].Track.Playable() {
		t.Error("the video kept is not playable")
	}
	a.renameBiliPlaylist(pl.ID, "上班路上")
	if got := a.biliPlaylist(pl.ID); got == nil || got.Name != "上班路上" {
		t.Fatalf("renamed %+v", got)
	}
	// Loaded afresh, the rows are playable again.
	b := newTestApp()
	b.loadBiliPlaylists()
	if len(b.biliPlaylists) != 1 || b.biliPlaylists[0].Items[0].Track.Name != "一首歌" {
		t.Fatalf("loaded %+v", b.biliPlaylists)
	}
	a.deleteBiliPlaylist(pl.ID)
	if len(a.biliPlaylists) != 0 {
		t.Errorf("delete left %+v", a.biliPlaylists)
	}
}

// The page lists the playlist's rows, and the sidebar opens it.
func TestBiliPlaylistPage(t *testing.T) {
	playlistFile(t)
	a := newTestApp()
	a.settings = defaultSettings()
	v := bilibili.Video{Bvid: "BV1xx411c7mD", Aid: 42, Title: "一首歌", Up: "UP主", Duration: 180000}
	pl := a.addBiliPlaylist("通勤", &v)
	a.show(route{page: pageBiliPlaylist, id: pl.ID})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if !tt.HasText("通勤") || !tt.HasText("一首歌") {
		t.Fatal("the playlist page does not show its name and row")
	}
	if err := tt.Click("一首歌"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if cur := a.pl.Current(); cur == nil || cur.Name != "一首歌" {
		t.Fatalf("current %+v, want the row clicked", cur)
	}
}

// A draft whose songs all matched makes a playlist of NetEase's songs,
// one a row.
func TestCreateDraftOfSongs(t *testing.T) {
	playlistFile(t)
	a := newTestApp()
	a.settings = defaultSettings()
	v := bilibili.Video{Bvid: "BV1xx411c7mD", Aid: 42, Title: "合集", Duration: 180000}
	hit := netease.Track{ID: 9, Name: "晴天", Artists: []netease.Artist{{Name: "周杰伦"}}, Duration: 269000}
	a.biliDraft = biliDraft{open: true, video: &v, cands: []biliCand{{Raw: "晴天 - 周杰伦", Track: &hit}}}
	a.show(route{page: pageBili})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("创建歌单"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(a.biliPlaylists) != 1 {
		t.Fatalf("playlists %+v", a.biliPlaylists)
	}
	pl := a.biliPlaylists[0]
	if pl.Mode != "songs" || len(pl.Items) != 1 || pl.Items[0].Track.ID != 9 {
		t.Fatalf("playlist %+v, want the matched song", pl)
	}
}

// A draft where some songs matched puts the video first, so the whole of
// what the video holds still plays, and the songs found after it.
func TestCreateDraftKeepsTheVideoFirst(t *testing.T) {
	playlistFile(t)
	a := newTestApp()
	a.settings = defaultSettings()
	v := bilibili.Video{Bvid: "BV1xx411c7mD", Aid: 42, Title: "合集", Duration: 180000}
	hit := netease.Track{ID: 9, Name: "晴天", Artists: []netease.Artist{{Name: "周杰伦"}}, Duration: 269000}
	a.biliDraft = biliDraft{open: true, video: &v, cands: []biliCand{
		{Raw: "晴天 - 周杰伦", Track: &hit},
		{Raw: "ICE - Dreams To Remember"},
	}}
	a.createBiliDraft()
	pl := a.biliPlaylists[0]
	if pl.Mode != "songs" || len(pl.Items) != 2 {
		t.Fatalf("playlist %+v, want the video and then the song", pl)
	}
	if pl.Items[0].Video == nil || pl.Items[0].Track.ID >= 0 {
		t.Fatalf("first %+v, want the video itself", pl.Items[0])
	}
	if pl.Items[1].Track.ID != 9 {
		t.Fatalf("second %+v, want the matched song", pl.Items[1])
	}
}
