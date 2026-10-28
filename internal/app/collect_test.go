package app

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// playingTestApp plays 宝船 with Now Playing shown, signed in with the
// playlists the page offers.
func playingTestApp() *app {
	a := editTestApp()
	a.pl.Cue([]netease.Track{a.playlists[10].Tracks[1]})
	a.nowPlaying = true
	return a
}

// nowPlayingFrame builds Now Playing, settled over the pages, and returns
// its tester.
func nowPlayingFrame(t *testing.T, a *app) *ui.Tester {
	t.Helper()
	return nowPlayingFrameAt(t, a, 1200, 800)
}

// nowPlayingFrameAt builds Now Playing in a window of w×h.
func nowPlayingFrameAt(t *testing.T, a *app, w, h int) *ui.Tester {
	t.Helper()
	tt := ui.NewTester(a.view, w, h)
	tt.Frame()
	tt.Advance(npSettle)
	tt.Frame()
	return tt
}

// The + of Now Playing lists the playlists the song playing may go into,
// and a click on one adds it there.
func TestAddThePlayingSongToAPlaylist(t *testing.T) {
	type add struct{ pid, song int64 }
	calls := make(chan add, 1)
	addToPlaylist = func(pid int64, songs ...int64) error { calls <- add{pid, songs[0]}; return nil }
	defer func() { addToPlaylist = netease.AddToPlaylist }()

	a := playingTestApp()
	tt := nowPlayingFrame(t, a)
	if err := tt.Click("收藏到歌单"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.collecting.open {
		t.Fatal("the playlists did not open")
	}
	for _, name := range []string{"创建新歌单", "小明喜欢的音乐", "通勤", "睡前"} {
		if _, ok := tt.Find(name); !ok {
			t.Errorf("%q is not among the playlists: %q", name, tt.Texts())
		}
	}
	if err := tt.Click("睡前"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := <-calls; got != (add{11, 2}) {
		t.Errorf("added %+v, want song 2 to playlist 11", got)
	}
	if a.collecting.open {
		t.Error("the playlists stayed open after one was chosen")
	}
}

// The liked songs among the playlists like the song, rather than add it to
// a playlist of the listener's.
func TestCollectLikesTheSongFromTheLikedSongs(t *testing.T) {
	calls := make(chan bool, 1)
	likeSong = func(id int64, like bool) error { calls <- like; return nil }
	defer func() { likeSong = netease.Like }()

	a := playingTestApp()
	tt := nowPlayingFrame(t, a)
	if err := tt.Click("收藏到歌单"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("小明喜欢的音乐"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := <-calls; !got {
		t.Error("unliked the song, want it liked")
	}
	if !a.liked[2] {
		t.Error("the song is not among the liked ones")
	}
}

// 创建新歌单 from the playlists asks for a name, the song to be put in the
// playlist made.
func TestCollectCreatesAPlaylistForTheSong(t *testing.T) {
	a := playingTestApp()
	tt := nowPlayingFrame(t, a)
	if err := tt.Click("收藏到歌单"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("创建新歌单"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.naming.open || a.naming.add == nil || a.naming.add.ID != 2 {
		t.Fatalf("the naming dialog is %+v, want it open to add song 2", a.naming)
	}
	if a.collecting.open {
		t.Error("the playlists stayed open under the naming dialog")
	}
}

// With NM_FRAMES set, saves the playing page's playlists and the page
// resting, to look at.
func TestCollectFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	save := func(tt *ui.Tester, name string) {
		tt.Advance(300 * time.Millisecond)
		tt.Frame()
		f, _ := os.Create(filepath.Join(dir, name+".png"))
		defer f.Close()
		png.Encode(f, tt.Image())
	}
	a := playingTestApp()
	tt := nowPlayingFrame(t, a)
	a.openCollect(a.pl.Current())
	save(tt, "collect")
	a.collecting.open = false
	tt.Advance(npIdle + 300*time.Millisecond)
	save(tt, "resting")
	// And in a window as narrow as the app may be.
	narrow := playingTestApp()
	nt := nowPlayingFrameAt(t, narrow, 880, 580)
	narrow.openCollect(narrow.pl.Current())
	save(nt, "collect-narrow")
}
