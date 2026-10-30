package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// Song actions copy the chosen value without changing playback.
func TestSongMenuCopies(t *testing.T) {
	var copied string
	real := copyText
	copyText = func(s string) error { copied = s; return nil }
	defer func() { copyText = real }()

	a := newTestApp()
	a.settings = defaultSettings()
	tr := netease.Track{ID: 1859245776, Name: "宝船", Artists: []netease.Artist{{Name: "DOUDOU"}, {Name: "小明"}}, Duration: 200000}
	a.playlists[42] = &netease.Playlist{ID: 42, Name: "飙升榜", Tracks: []netease.Track{tr}}
	a.route = route{page: pagePlaylist, id: 42}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()

	// The song's menu is its row's, by a right click: the bar's [...] is
	// the panel of the queue and the comments.
	for _, c := range []struct{ item, want string }{
		{"复制歌名", "宝船"},
		{"复制歌名和歌手", "宝船 - DOUDOU, 小明"},
		{"复制链接", "https://music.163.com/song?id=1859245776"},
	} {
		if err := tt.RightClick("宝船"); err != nil {
			t.Fatal(err)
		}
		tt.Advance(300 * time.Millisecond)
		tt.Frame()
		if err := tt.Click(c.item); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if copied != c.want {
			t.Errorf("%s copied %q, want %q", c.item, copied, c.want)
		}
	}
}

// Right clicking opens song actions without playing the row.
func TestSongMenuOnRightClick(t *testing.T) {
	var copied string
	real := copyText
	copyText = func(s string) error { copied = s; return nil }
	defer func() { copyText = real }()

	a := newTestApp()
	a.settings = defaultSettings()
	a.playlists[42] = &netease.Playlist{ID: 42, Name: "飙升榜", Tracks: []netease.Track{
		{ID: 1, Name: "一张相片", Artists: []netease.Artist{{Name: "林宥嘉"}}, Duration: 200000},
		{ID: 2, Name: "宝船", Artists: []netease.Artist{{Name: "DOUDOU"}}, Duration: 200000},
	}}
	a.route = route{page: pagePlaylist, id: 42}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()

	if err := tt.RightClick("宝船"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(300 * time.Millisecond)
	tt.Frame()

	if err := tt.Click("复制链接"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if want := "https://music.163.com/song?id=2"; copied != want {
		t.Errorf("copied %q, want %q", copied, want)
	}
	if a.pl.Current() != nil {
		t.Error("the right click played the song")
	}
}

// The visible song button toggles its menu, and Escape dismisses it without playing.
func TestSongMenuButtonAndDismiss(t *testing.T) {
	a := editTestApp()
	tt := ui.NewTester(a.view, 880, 580)
	tt.Frame()
	for range 2 {
		if err := tt.Click("歌曲操作：宝船"); err != nil {
			t.Fatal(err)
		}
		tt.Advance(300 * time.Millisecond)
		tt.Frame()
	}
	if _, ok := tt.Find("复制链接"); ok {
		t.Fatal("the second click left the menu open")
	}
	if err := tt.Click("歌曲操作：宝船"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(300 * time.Millisecond)
	tt.Frame()
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if _, ok := tt.Find("复制链接"); ok {
		t.Error("Escape left the menu open")
	}
	if a.pl.Current() != nil {
		t.Error("the song action button played the row")
	}
}
