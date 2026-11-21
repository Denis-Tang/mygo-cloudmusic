package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/bilibili"
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

// A card of the feed opens the playlist its video makes, as a preview
// that leaves the sidebar and the store alone.
func TestBiliFeedCardOpensPlaylist(t *testing.T) {
	followsFile(t)
	playlistFile(t)
	a := newTestApp()
	a.settings = defaultSettings()
	a.follows = []biliFollow{{Mid: 7, Name: "UP主"}}
	v := bilibili.Video{Bvid: "BV1xx411c7mD", Aid: 42, CID: 7, Title: "一首歌", Up: "UP主", UpMid: 7, Duration: 3 * time.Minute, PubDate: 100}
	a.addFeedVideo(v)
	a.rebuildFeedTracks()
	pl := a.makeBiliPlaylist(v, nil, false)
	a.biliPreview = &pl
	a.show(route{page: pageBili})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("一首歌"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.route.page != pageBiliPlaylist || a.route.id != pl.ID {
		t.Fatalf("route %+v, want the video's playlist", a.route)
	}
	if len(a.biliPlaylists) != 0 {
		t.Fatalf("playlists %+v, want the preview left out", a.biliPlaylists)
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

// The clear button beside the query empties what was typed, and the field
// keeps the focus.
func TestBiliSearchFieldClears(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.show(route{page: pageBili})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("搜索哔哩哔哩"); err != nil {
		t.Fatal(err)
	}
	tt.Type("一段很长很长的搜索文字")
	tt.Frame()
	if a.biliQuery == "" {
		t.Fatal("the field took nothing")
	}
	if err := tt.Click("清除搜索"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.biliQuery != "" {
		t.Fatalf("query %q, want it cleared", a.biliQuery)
	}
	if !tt.Focused("搜索哔哩哔哩") {
		t.Error("the field lost the focus")
	}
}

// Before an uploader is followed the page says so, and its search field is
// the way to find one.
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
	if err := tt.Click("搜索哔哩哔哩"); err != nil {
		t.Fatal(err)
	}
}

// The uploader followed can be let go from the strip of faces, which shows
// the way under the pointer.
func TestBiliFollowStripUnfollows(t *testing.T) {
	followsFile(t)
	a := newTestApp()
	a.settings = defaultSettings()
	a.follows = []biliFollow{{Mid: 7, Name: "音乐UP"}}
	a.show(route{page: pageBili})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	r, ok := tt.Find("音乐UP")
	if !ok {
		t.Fatal("the uploader followed is not on the page")
	}
	tt.Move(r.X+r.W/2, r.Y+r.H/2)
	tt.Frame()
	if err := tt.Click("取消关注 音乐UP"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(a.follows) != 0 {
		t.Fatalf("follows %+v, want none", a.follows)
	}
}

// Unfollowing the first of several uploaders does not mutate the slice
// during the build pass and duplicate keys for the remaining faces.
func TestBiliFollowStripUnfollowsFirstOfMany(t *testing.T) {
	followsFile(t)
	a := newTestApp()
	a.settings = defaultSettings()
	a.follows = []biliFollow{
		{Mid: 7, Name: "甲UP"},
		{Mid: 8, Name: "乙UP"},
		{Mid: 9, Name: "丙UP"},
	}
	a.show(route{page: pageBili})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	r, ok := tt.Find("甲UP")
	if !ok {
		t.Fatal("first uploader missing")
	}
	tt.Move(r.X+r.W/2, r.Y+r.H/2)
	tt.Frame()
	if err := tt.Click("取消关注 甲UP"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(a.follows) != 2 || a.follows[0].Mid != 8 || a.follows[1].Mid != 9 {
		t.Fatalf("follows %+v, want [8, 9]", a.follows)
	}
}

// A face of the strip lists that uploader's playlists alone, and another
// click lists everybody's again.
func TestBiliFilterByFace(t *testing.T) {
	followsFile(t)
	playlistFile(t)
	a := newTestApp()
	a.settings = defaultSettings()
	a.follows = []biliFollow{{Mid: 7, Name: "甲UP"}, {Mid: 8, Name: "乙UP"}}
	a.biliFeed = []bilibili.Video{
		{Bvid: "BV1", UpMid: 7, Up: "甲UP", Title: "甲的歌单", PubDate: 300},
		{Bvid: "BV2", UpMid: 8, Up: "乙UP", Title: "乙的歌单", PubDate: 299},
	}
	a.show(route{page: pageBili})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("查看 甲UP 的歌单"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if _, ok := tt.Find("乙的歌单"); ok {
		t.Fatal("another uploader's playlist stayed")
	}
	if _, ok := tt.Find("甲的歌单"); !ok {
		t.Fatal("the uploader's playlist went")
	}
	if err := tt.Click("查看 甲UP 的歌单"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if _, ok := tt.Find("乙的歌单"); !ok {
		t.Fatal("all the playlists did not come back")
	}
}

// An uploader search lists whom it found, and its button follows or stops
// following them.
func TestBiliFollowFromResults(t *testing.T) {
	followsFile(t)
	u := bilibili.User{Mid: 9, Name: "音乐UP", Face: "https://i0.hdslb.com/a.jpg", Fans: 12}
	a := newTestApp()
	a.settings = defaultSettings()
	a.biliShown, a.biliQuery = "音乐", "音乐"
	a.biliUsers = []bilibili.User{u}
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
	if err := tt.Click("取消关注"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.followed(9) {
		t.Fatalf("follows %+v, want the uploader let go", a.follows)
	}
}

// A video searched for shows over the feed of the uploaders followed, so a
// link or a BV number still opens what it names.
func TestSearchedVideoShowsOverFeed(t *testing.T) {
	followsFile(t)
	playlistFile(t)
	a := newTestApp()
	a.settings = defaultSettings()
	a.follows = []biliFollow{{Mid: 7, Name: "音乐UP"}}
	v := bilibili.Video{Bvid: "BV1xx411c7mD", Aid: 42, Title: "一首歌", Up: "UP", Duration: 3 * time.Minute}
	pl := a.makeBiliPlaylist(v, nil, false)
	a.biliPreview = &pl
	a.biliShown, a.biliHits = v.Bvid, []bilibili.Video{v}
	a.show(route{page: pageBili})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("一首歌"); err != nil {
		t.Fatalf("the searched video is not listed: %v", err)
	}
	tt.Frame()
	if a.route.page != pageBiliPlaylist || a.route.id != pl.ID {
		t.Fatalf("route %+v, want the video's playlist", a.route)
	}
}
