package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/bilibili"
	"netease-music-mygo/internal/netease"
)

// A dragged playlist moves within its own group: an imported collection
// stays above the ones collected here.
func TestMoveBiliPlaylistStaysInItsGroup(t *testing.T) {
	playlistFile(t)
	a := newTestApp()
	a.biliPlaylists = []biliPlaylist{
		{ID: 1, Name: "导入甲", Remote: 100, Collected: true},
		{ID: 2, Name: "收藏甲", Collected: true},
		{ID: 3, Name: "收藏乙", Collected: true},
	}
	a.moveBiliPlaylist(3, -1, false)
	got := [3]int64{a.biliPlaylists[0].ID, a.biliPlaylists[1].ID, a.biliPlaylists[2].ID}
	if got != [3]int64{1, 3, 2} {
		t.Fatalf("order %v, want the two collected swapped", got)
	}
	a.moveBiliPlaylist(3, -1, false)
	if a.biliPlaylists[0].ID != 1 {
		t.Fatalf("a collected playlist went above one imported: %+v", a.biliPlaylists)
	}
	a.moveBiliPlaylist(1, 1, true)
	if a.biliPlaylists[0].ID != 1 {
		t.Fatalf("the only imported playlist moved: %+v", a.biliPlaylists)
	}
}

// The site's playlists keep the order they were dragged to, on this
// machine alone; the one the site keeps apart stays at the head.
func TestMoveSitePlaylistKeepsOrderHere(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.mine = []netease.Playlist{
		{ID: 10, Name: "我喜欢的音乐", SpecialType: 5},
		{ID: 11, Name: "甲"},
		{ID: 12, Name: "乙"},
	}
	a.moveSitePlaylist(true, 12, -1)
	if a.mine[1].ID != 12 || a.mine[2].ID != 11 {
		t.Fatalf("order %+v, want 12 before 11", a.mine)
	}
	if a.mine[0].ID != 10 {
		t.Fatalf("the liked songs left the head: %+v", a.mine)
	}
	if len(a.settings.MineOrder) != 3 || a.settings.MineOrder[1] != 12 {
		t.Fatalf("kept order %v", a.settings.MineOrder)
	}
	// Loaded from the site afresh, the order kept here is put back.
	fromSite := []netease.Playlist{{ID: 11}, {ID: 10, SpecialType: 5}, {ID: 12}}
	ordered := orderPlaylists(fromSite, a.settings.MineOrder)
	if ordered[0].ID != 10 || ordered[1].ID != 12 || ordered[2].ID != 11 {
		t.Fatalf("ordered %+v, want the kept order above the site's", ordered)
	}
}

// A hold and a drag in the sidebar moves a playlist below the one after
// it, and opens nothing.
func TestBiliPlaylistDragsToReorder(t *testing.T) {
	playlistFile(t)
	a := newTestApp()
	a.settings = defaultSettings()
	a.mineSrc = mineBili
	v := bilibili.Video{Bvid: "BV1xx411c7mD", Aid: 42, Title: "一首歌", Duration: 180000}
	for _, name := range []string{"甲歌单", "乙歌单"} {
		pl := a.makeBiliPlaylist(v, nil, false)
		pl.Name, pl.Collected = name, true
		a.biliPlaylists = append(a.biliPlaylists, pl)
	}
	old := dragHold
	dragHold = 0
	t.Cleanup(func() { dragHold = old })
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	r, ok := tt.Find("甲歌单")
	if !ok {
		t.Fatal("the playlist row is not listed")
	}
	tt.Press(r.X+r.W/2, r.Y+r.H/2)
	tt.Frame()
	tt.Frame() // held long enough: the row may be dragged
	tt.Move(r.X+r.W/2, r.Y+r.H/2+rowPitch+6)
	tt.Frame()
	tt.Release(r.X+r.W/2, r.Y+r.H/2+rowPitch+6)
	tt.Frame()
	if a.biliPlaylists[0].Name != "乙歌单" || a.biliPlaylists[1].Name != "甲歌单" {
		t.Fatalf("order %q, %q, want them swapped", a.biliPlaylists[0].Name, a.biliPlaylists[1].Name)
	}
	if a.route.page == pageBiliPlaylist {
		t.Fatal("dragging the row opened it")
	}
}

// A hold and a drag in the sidebar moves one of the site's playlists too,
// and the order is kept here.
func TestSitePlaylistDragsToReorder(t *testing.T) {
	a := editTestApp()
	a.mine = []netease.Playlist{{ID: 11, Name: "甲歌单"}, {ID: 12, Name: "乙歌单"}}
	old := dragHold
	dragHold = 0
	t.Cleanup(func() { dragHold = old })
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	r, ok := tt.Find("甲歌单")
	if !ok {
		t.Fatal("the playlist row is not listed")
	}
	tt.Press(r.X+r.W/2, r.Y+r.H/2)
	tt.Frame()
	tt.Frame()
	tt.Move(r.X+r.W/2, r.Y+r.H/2+rowPitch+6)
	tt.Frame()
	tt.Release(r.X+r.W/2, r.Y+r.H/2+rowPitch+6)
	tt.Frame()
	if a.mine[0].Name != "乙歌单" || a.mine[1].Name != "甲歌单" {
		t.Fatalf("order %q, %q, want them swapped", a.mine[0].Name, a.mine[1].Name)
	}
}
