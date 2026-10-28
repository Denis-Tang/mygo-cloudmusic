package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// coverApp is Now Playing on the second of three songs, the cover state,
// where the song's bar shows once the pointer is over the window.
func coverApp(t *testing.T) (*app, *ui.Tester, ui.Rect) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.pl.Cue([]netease.Track{{ID: 1, Name: "一"}, {ID: 2, Name: "二"}, {ID: 3, Name: "三"}})
	a.pl.Next()
	a.pl.SetLyrics(nil)
	a.nowPlaying, a.npPane = true, paneCover
	tt := ui.NewTester(a.view, 860, 560)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tt.Frame()
	tt.Advance(500 * time.Millisecond)
	tt.Move(430, 280) // over the picture: the tools rise, and stay a while
	tt.Frame()
	r, ok := tt.Find("播放进度")
	if !ok {
		t.Fatal("no progress over the playing page")
	}
	return a, tt, r
}
