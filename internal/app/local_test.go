package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/player"
)

// A track of the listener's own folders plays when its row is clicked, as
// NetEase's songs do.
func TestLocalTrackPlays(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.libReady = true
	a.pl.LocalOf = func(id int64) (player.Local, bool) { return player.Local{}, false }
	a.locals = []netease.Track{{
		ID:        -5,
		Name:      "本地之歌",
		Artists:   []netease.Artist{{Name: "歌手"}},
		Duration:  180000,
		Privilege: &netease.Privilege{Pl: 999000},
	}}
	a.show(route{page: pageLocal})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("本地之歌"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if cur := a.pl.Current(); cur == nil || cur.ID != -5 {
		t.Fatalf("current track %+v, want the local one", cur)
	}
}

// The page says where music comes from while there is none.
func TestLocalPageWithoutFolders(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.libReady = true
	a.show(route{page: pageLocal})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if a.route.page != pageLocal {
		t.Fatalf("page %v, want the local one", a.route.page)
	}
}
