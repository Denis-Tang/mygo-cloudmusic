package app

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"netease-music-mygo/internal/bilibili"
)

// followsFile puts the store in the test's folder, and restores it after.
func followsFile(t *testing.T) {
	t.Helper()
	old := biliFollowsFile
	dir := t.TempDir()
	biliFollowsFile = func() (string, error) { return filepath.Join(dir, "follows.json"), nil }
	t.Cleanup(func() { biliFollowsFile = old })
}

func stubUsers(t *testing.T, users ...bilibili.User) {
	t.Helper()
	old := biliSearchUsers
	biliSearchUsers = func(ctx context.Context, q string, page int) ([]bilibili.User, error) { return users, nil }
	t.Cleanup(func() { biliSearchUsers = old })
}

// Following an uploader is kept on this machine, and the videos the search
// carried are not marked new.
func TestFollowKeepsOnThisMachine(t *testing.T) {
	followsFile(t)
	stubUsers(t)
	a := newTestApp()
	a.settings = defaultSettings()
	u := bilibili.User{Mid: 7, Name: "音乐UP", Face: "face", Latest: []bilibili.Video{
		{Bvid: "BV1", Aid: 1, Title: "旧歌", PubDate: 100},
	}}
	a.followBili(u)
	if len(a.follows) != 1 || a.follows[0].Mid != 7 {
		t.Fatalf("follows %+v", a.follows)
	}
	if got := loadBiliFollows(); len(got) != 1 || got[0].Seen[0] != "BV1" {
		t.Fatalf("stored %+v, want the video seen already", got)
	}
	if a.biliNewNum != 0 || a.biliNew["BV1"] {
		t.Error("the video the search showed came in as new")
	}
	a.unfollowBili(7)
	if len(loadBiliFollows()) != 0 {
		t.Error("unfollowing was not kept")
	}
}

// A video posted after the last look is new, until the page is opened.
func TestFeedMarksNew(t *testing.T) {
	followsFile(t)
	stubUsers(t, bilibili.User{Mid: 7, Name: "音乐UP", Latest: []bilibili.Video{
		{Bvid: "BV1", Aid: 1, Title: "旧歌", PubDate: 100},
		{Bvid: "BV2", Aid: 2, Title: "新歌", PubDate: 200},
	}})
	a := newTestApp()
	a.follows = []biliFollow{{Mid: 7, Name: "音乐UP", Seen: []string{"BV1"}}}
	a.biliNew = map[string]bool{"BV2": true}
	a.addFeedVideo(bilibili.Video{Bvid: "BV1", Aid: 1, PubDate: 100})
	a.addFeedVideo(bilibili.Video{Bvid: "BV2", Aid: 2, PubDate: 200})
	a.rebuildFeedTracks()
	if a.biliNewNum != 0 { // recount only after a refresh
		t.Fatalf("count before refresh %d", a.biliNewNum)
	}
	if a.biliFeed[0].Bvid != "BV2" {
		t.Fatalf("feed order %+v, want newest first", a.biliFeed)
	}
	a.biliNewNum = a.countNew()
	if a.biliNewNum != 1 {
		t.Fatalf("new %d, want 1", a.biliNewNum)
	}
	a.markBiliSeen()
	if a.biliNewNum != 0 || len(a.biliNew) != 0 {
		t.Error("the badge did not go")
	}
	if len(a.follows[0].Seen) != 2 {
		t.Fatalf("seen %v, want both videos", a.follows[0].Seen)
	}
}

// The feed filters to one uploader, keeping the order of the whole.
func TestFeedFilter(t *testing.T) {
	a := newTestApp()
	a.follows = []biliFollow{{Mid: 7}, {Mid: 8}}
	a.addFeedVideo(bilibili.Video{Bvid: "BV1", Aid: 1, UpMid: 7, PubDate: 100})
	a.addFeedVideo(bilibili.Video{Bvid: "BV2", Aid: 2, UpMid: 8, PubDate: 200})
	a.rebuildFeedTracks()
	if len(a.feedTracks()) != 2 {
		t.Fatalf("all: %d", len(a.feedTracks()))
	}
	a.biliFilter = 7
	if got := a.feedTracks(); len(got) != 1 || got[0].Name != a.biliFeed[1].Title {
		t.Fatalf("filtered %+v", got)
	}
	_ = time.Now
}
