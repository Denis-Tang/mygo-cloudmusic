package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/bilibili"
)

// accountFile puts the account in the test's folder, and restores it after.
func accountFile(t *testing.T) {
	t.Helper()
	old := biliAccountFile
	dir := t.TempDir()
	biliAccountFile = func() (string, error) { return filepath.Join(dir, "account.json"), nil }
	t.Cleanup(func() {
		biliAccountFile = old
		bilibili.SetSession("")
	})
}

// The account signed in is kept for the next start, signed into the site's
// calls, and forgotten on sign-out.
func TestBiliAccountIsKeptAndForgotten(t *testing.T) {
	accountFile(t)
	a := newTestApp()
	a.saveBiliAccount(&biliAccount{Cookie: "SESSDATA=abc; bili_jct=def", Mid: 7, Name: "听众"})
	if got := bilibili.SessionCookie(); !strings.Contains(got, "SESSDATA=abc") {
		t.Fatalf("site cookie %q, want the account's", got)
	}
	b := newTestApp()
	b.loadBiliAccount()
	if b.biliAcc == nil || b.biliAcc.Mid != 7 || b.biliAcc.Name != "听众" {
		t.Fatalf("loaded %+v", b.biliAcc)
	}
	b.saveBiliAccount(nil)
	if b.biliAcc != nil || bilibili.SessionCookie() != "" {
		t.Fatal("signing out kept the account")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(mustAccountPath(t)), "account.json")); !os.IsNotExist(err) {
		t.Fatal("signing out kept the file")
	}
}

func mustAccountPath(t *testing.T) string {
	t.Helper()
	path, err := biliAccountFile()
	if err != nil {
		t.Fatal(err)
	}
	return path
}

// Importing a collection makes a playlist of its videos, and importing it
// again updates that one rather than adds another.
func TestImportBiliFolderKeepsItOnce(t *testing.T) {
	playlistFile(t)
	a := newTestApp()
	a.settings = defaultSettings()
	long := strings.Repeat("长", 60)
	f := bilibili.FavFolder{ID: 900, Name: long, Count: 2, Cover: "https://i0.hdslb.com/a.jpg"}
	videos := []bilibili.FavVideo{
		{Bvid: "BV1xx411c7mD", Aid: 42, Title: "第一首", Up: "UP主", UpMid: 7, Duration: 180},
		{Bvid: "BV1xx411c7mE", Aid: 43, Title: "第二首", Up: "UP主", UpMid: 7, Duration: 200},
	}
	a.keepBiliImport(f, videos)
	if len(a.biliPlaylists) != 1 {
		t.Fatalf("playlists %+v", a.biliPlaylists)
	}
	pl := a.biliPlaylists[0]
	if !pl.Collected || pl.Remote != f.ID || pl.Name != long || len(pl.Items) != 2 {
		t.Fatalf("playlist %+v, want the collection kept whole", pl)
	}
	if got := a.biliKept(); len(got) != 1 || got[0] != &a.biliPlaylists[0] {
		t.Fatalf("kept %+v, want the imported playlist", got)
	}
	a.keepBiliImport(f, videos[:1])
	if len(a.biliPlaylists) != 1 || len(a.biliPlaylists[0].Items) != 1 {
		t.Fatalf("playlists %+v, want the same one updated", a.biliPlaylists)
	}
}

// A new search drops the preview the last video made, so the same video
// makes a fresh one when it is opened again.
func TestSearchDropsThePreview(t *testing.T) {
	playlistFile(t)
	a := newTestApp()
	a.settings = defaultSettings()
	v := bilibili.Video{Bvid: "BV1xx411c7mD", Aid: 42, Title: "一首歌", Duration: 180000}
	pl := a.makeBiliPlaylist(v, nil, false)
	a.biliPreview = &pl
	oldSearch, oldUsers := biliSearch, biliSearchUsers
	biliSearch = func(context.Context, string, int) ([]bilibili.Video, error) { return nil, nil }
	biliSearchUsers = func(context.Context, string, int) ([]bilibili.User, error) { return nil, nil }
	t.Cleanup(func() { biliSearch, biliSearchUsers = oldSearch, oldUsers })
	a.loadBili("周杰伦")
	if a.biliPreview != nil {
		t.Fatal("a new search kept the preview")
	}
}

// A collection imported from the account is played and renamed here, but
// is not made again from one of its videos.
func TestImportedCollectionHasNoRebuild(t *testing.T) {
	playlistFile(t)
	a := newTestApp()
	a.settings = defaultSettings()
	f := bilibili.FavFolder{ID: 5, Name: "收藏夹"}
	a.keepBiliImport(f, []bilibili.FavVideo{{Bvid: "BV1xx411c7mD", Aid: 1, Title: "一首歌", Up: "UP主", UpMid: 7}})
	a.show(route{page: pageBiliPlaylist, id: a.biliPlaylists[0].ID})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if _, ok := tt.Find("播放"); !ok {
		t.Fatal("the imported collection has no play button")
	}
	if _, ok := tt.Find("随机播放"); !ok {
		t.Fatal("the imported collection has no shuffle button")
	}
	if _, ok := tt.Find("重命名"); ok {
		t.Fatal("the head is a row of buttons, not one menu")
	}
	if err := tt.Click("更多操作"); err != nil {
		t.Fatalf("no menu of the rest: %v", err)
	}
	tt.Frame()
	if _, ok := tt.Find("重命名…"); !ok {
		t.Fatal("the menu has no rename")
	}
	if _, ok := tt.Find("重新生成"); ok {
		t.Fatal("an imported collection offers to be made again from one video")
	}
}

// A playlist a video made offers to be made again from it, under the same
// menu.
func TestVideoPlaylistOffersToBeMadeAgain(t *testing.T) {
	playlistFile(t)
	a := newTestApp()
	a.settings = defaultSettings()
	v := bilibili.Video{Bvid: "BV1xx411c7mD", Aid: 42, Title: "一首歌", Duration: 180000}
	pl := a.makeBiliPlaylist(v, nil, false)
	pl.Collected = true
	a.biliPlaylists = []biliPlaylist{pl}
	a.show(route{page: pageBiliPlaylist, id: pl.ID})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("更多操作"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if _, ok := tt.Find("重新生成"); !ok {
		t.Fatal("a video's playlist offers no way to be made again")
	}
}

// The uploaders followed on the account are added once each here.
func TestImportBiliFollowsAddsEachOnce(t *testing.T) {
	followsFile(t)
	a := newTestApp()
	users := []bilibili.User{{Mid: 7, Name: "甲"}, {Mid: 8, Name: "乙"}, {Mid: 7, Name: "甲"}}
	if added := a.keepBiliFollows(users); added != 2 || len(a.follows) != 2 {
		t.Fatalf("added %d, follows %+v", added, a.follows)
	}
	if added := a.keepBiliFollows(users); added != 0 || len(a.follows) != 2 {
		t.Fatalf("added %d again, follows %+v", added, a.follows)
	}
}

// A video's menu has one way to collect it, the one that reads its songs.
func TestBiliSongMenuHasOneCollectItem(t *testing.T) {
	playlistFile(t)
	a := newTestApp()
	a.settings = defaultSettings()
	v := bilibili.Video{Bvid: "BV1xx411c7mD", Aid: 42, Title: "一首歌", Up: "UP主", Duration: 180000}
	pl := a.makeBiliPlaylist(v, nil, false)
	pl.Collected = true
	a.biliPlaylists = []biliPlaylist{pl}
	a.show(route{page: pageBiliPlaylist, id: pl.ID})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("歌曲操作：一首歌"); err != nil {
		t.Fatalf("no menu for the video: %v", err)
	}
	tt.Frame()
	if _, ok := tt.Find("收藏为歌单"); !ok {
		t.Fatal("the menu has no way to collect the video")
	}
	if _, ok := tt.Find("生成歌单…"); ok {
		t.Fatal("the menu still has a second way to make a playlist")
	}
}
