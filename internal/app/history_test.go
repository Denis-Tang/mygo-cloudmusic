package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// A newer operation replaces the previous feedback and expires, rather
// than stacking contradictory messages as the default toast IDs did.
func TestStatusToastReplacesPreviousMessage(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.toast = "first operation"
	tt := ui.NewTester(a.view, 1000, 700)
	tt.Frame()
	tt.Advance(300 * time.Millisecond)
	tt.Frame()
	a.toast = "second operation"
	tt.Frame()
	tt.Advance(300 * time.Millisecond)
	tt.Frame()
	if tt.HasText("first operation") || !tt.HasText("second operation") {
		t.Fatal("the new message did not replace the old message")
	}
	tt.Advance(3 * time.Second)
	tt.Frame()
	if tt.HasText("second operation") {
		t.Fatal("the message did not expire")
	}
}

// The side buttons of a mouse go back and forward between pages.
func TestBackAndForward(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.playlists[42] = &netease.Playlist{ID: 42, Name: "飙升榜", Tracks: []netease.Track{{ID: 1, Name: "一张相片"}}}
	tt := ui.NewTester(a.view, 1200, 800)
	charts, list := route{page: pageCharts}, route{page: pagePlaylist, id: 42}
	a.open(charts)
	a.open(list)
	tt.Frame()

	tt.Key(0, ui.KeyBack)
	tt.Frame()
	if a.route != charts {
		t.Fatalf("back went to %+v", a.route)
	}
	tt.Key(0, ui.KeyForward)
	tt.Frame()
	if a.route != list {
		t.Fatalf("forward went to %+v", a.route)
	}

	// Back closes Now Playing before it leaves the page under it.
	a.pl.Cue([]netease.Track{{ID: 1, Name: "一张相片"}})
	a.nowPlaying = true
	tt.Frame()
	tt.Key(0, ui.KeyBack)
	tt.Frame()
	if a.nowPlaying || a.route != list {
		t.Fatalf("back with Now Playing open: open %v, page %+v", a.nowPlaying, a.route)
	}

	// A new page leaves nothing to go forward to.
	tt.Key(0, ui.KeyBack)
	tt.Frame()
	a.open(route{page: pageHome})
	tt.Key(0, ui.KeyForward)
	tt.Frame()
	if a.route != (route{page: pageHome}) {
		t.Fatalf("forward after a new page went to %+v", a.route)
	}
}
