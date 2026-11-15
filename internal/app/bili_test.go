package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/bilibili"
	"netease-music-mygo/internal/netease"
)

// A video of Bilibili is listed as a song: its title, its uploader and its
// cover, with an ID of its own.
func TestBiliTrack(t *testing.T) {
	a := newTestApp()
	v := bilibili.Video{Bvid: "BV1xx411c7mD", Aid: 42, CID: 7, Title: "一首歌", Up: "UP主", Cover: "https://i0.hdslb.com/a.jpg", Duration: 3 * time.Minute}
	tr := a.biliTrack(v)
	if tr.ID >= 0 || tr.Name != v.Title || tr.Duration != 180000 {
		t.Fatalf("track %+v, want the video's title and length", tr)
	}
	if len(tr.Artists) != 1 || tr.Artists[0].Name != v.Up {
		t.Fatalf("artists %+v, want the uploader", tr.Artists)
	}
	if _, ok := a.biliVideos[tr.ID]; !ok {
		t.Error("video not kept for its menu")
	}
	if _, ok := a.biliRemotes[tr.ID]; !ok {
		t.Error("no way found to play it")
	}
	if !tr.Playable() {
		t.Error("a video should be playable")
	}
}

// The feed lists what the uploaders followed posted, and a click plays the
// video clicked.
func TestBiliPagePlays(t *testing.T) {
	followsFile(t)
	a := newTestApp()
	a.settings = defaultSettings()
	a.follows = []biliFollow{{Mid: 7, Name: "UP主"}}
	a.addFeedVideo(bilibili.Video{Bvid: "BV1xx411c7mD", Aid: 42, CID: 7, Title: "一首歌", Up: "UP主", UpMid: 7, Duration: 3 * time.Minute, PubDate: 100})
	a.rebuildFeedTracks()
	a.show(route{page: pageBili})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("一首歌"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if cur := a.pl.Current(); cur == nil || cur.Name != "一首歌" {
		t.Fatalf("current %+v, want the video clicked", cur)
	}
}

// Words are searched for; a link, a BV or an av number names one video.
func TestBiliLink(t *testing.T) {
	for q, want := range map[string]bool{
		"周杰伦":                                    false,
		"avril lavigne":                            false,
		"BV1xx411c7mD":                             true,
		"av170001":                                 true,
		"https://www.bilibili.com/video/BV1xx411c7mD": true,
		"b23.tv/abc":                               true,
	} {
		if got := biliLink(q); got != want {
			t.Errorf("%q: link %v, want %v", q, got, want)
		}
	}
}

// The search field takes what is typed into it, and Enter searches.
func TestBiliSearchFieldTypes(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.show(route{page: pageBili})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("搜索哔哩哔哩"); err != nil {
		t.Fatal(err)
	}
	tt.Type("晴天")
	tt.Frame()
	if a.biliQuery != "晴天" {
		t.Fatalf("query %q, want what was typed", a.biliQuery)
	}
}

// Before an uploader is followed the page says so, and its button turns the
// search to uploaders.
func TestBiliEmptyState(t *testing.T) {
	followsFile(t)
	a := newTestApp()
	a.settings = defaultSettings()
	a.show(route{page: pageBili})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if !tt.HasText("还没有关注 UP 主") {
		t.Fatal("the empty page does not say how to fill it")
	}
	if err := tt.Click("去搜索 UP 主"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.biliMode != biliUploaders {
		t.Fatalf("mode %v, want uploaders", a.biliMode)
	}
}

// An uploader search lists whom it found, and following one keeps it here.
func TestBiliFollowFromResults(t *testing.T) {
	followsFile(t)
	stubUsers(t, bilibili.User{Mid: 9, Name: "音乐UP", Face: "https://i0.hdslb.com/a.jpg", Fans: 12})
	a := newTestApp()
	a.settings = defaultSettings()
	a.biliMode = biliUploaders
	a.biliUserQ = "音乐"
	a.biliUsers = []bilibili.User{{Mid: 9, Name: "音乐UP", Face: "https://i0.hdslb.com/a.jpg", Fans: 12}}
	a.show(route{page: pageBili})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("关注"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.followed(9) {
		t.Fatalf("follows %+v, want the uploader clicked", a.follows)
	}
	if len(loadBiliFollows()) != 1 {
		t.Error("the follow was not kept")
	}
}

// A video searched for shows over the feed of the uploaders followed, so a
// link or a BV number still plays what it names.
func TestSearchedVideoShowsOverFeed(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.follows = []biliFollow{{Mid: 7, Name: "音乐UP"}}
	v := bilibili.Video{Bvid: "BV1xx411c7mD", Aid: 42, Title: "一首歌", Up: "UP", Duration: 3 * time.Minute}
	a.biliShown, a.biliTracks = v.Bvid, []netease.Track{a.biliTrack(v)}
	a.show(route{page: pageBili})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("一首歌"); err != nil {
		t.Fatalf("the searched video is not listed: %v", err)
	}
	tt.Frame()
	if cur := a.pl.Current(); cur == nil || cur.Name != "一首歌" {
		t.Fatalf("current %+v, want the video clicked", cur)
	}
}
