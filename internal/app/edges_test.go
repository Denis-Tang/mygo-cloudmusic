package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// playTracks builds a playlist of n songs, the page the edges over.
func playTracks(n int) *netease.Playlist {
	pl := &netease.Playlist{ID: 7, Name: "歌单"}
	for i := range n {
		pl.Tracks = append(pl.Tracks, netease.Track{ID: int64(i + 1), Name: "歌"})
	}
	return pl
}

func TestScrollEdgesFollowContent(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.playlists[7] = playTracks(40)
	a.open(route{page: pagePlaylist, id: 7})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if a.underTop {
		t.Error("top edge shows before scrolling")
	}
	tt.Scroll(800, 400, 0, 100000)
	tt.Advance(time.Second)
	tt.Frame()
	tt.Frame()
	if !a.underTop {
		t.Error("no top edge after scrolling")
	}
}

func TestShortPageHasNoEdges(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.playlists[7] = playTracks(3)
	a.open(route{page: pagePlaylist, id: 7})
	tt := ui.NewTester(a.view, 1200, 1400)
	tt.Frame()
	if a.underTop {
		t.Error("top edge shows on a page that fits")
	}
}
