package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// The sidebar toggle stays under the pointer in both states; its two
// previous containers put it at different window coordinates.
func TestSidebarToggleStaysUnderPointer(t *testing.T) {
	for _, width := range []int{880, 1180} {
		a := catalogApp()
		a.route = route{page: pageAlbum, id: 8}
		a.history = []route{{page: pageHome}}
		tt := ui.NewTester(a.view, width, 780)
		tt.Frame()
		r, ok := tt.Find("收起侧栏")
		if !ok {
			t.Fatal("no sidebar toggle")
		}
		x, y := r.X+r.W/2, r.Y+r.H/2
		for i := range 4 {
			tt.ClickAt(x, y)
			tt.Advance(time.Second)
			tt.Frame()
			want := i%2 != 0
			if a.settings.Sidebar != want {
				t.Fatalf("width %d: click %d did not toggle the sidebar", width, i)
			}
			label := "展开侧栏"
			if want {
				label = "收起侧栏"
			}
			got, ok := tt.Find(label)
			if !ok || got.X+got.W/2 != x || got.Y+got.H/2 != y {
				t.Fatalf("width %d: toggle moved from (%g,%g) to %+v", width, x, y, got)
			}
		}
	}
}

// Changing an artist's tab keeps the header in place; track pages
// previously started 12 points left of the album and event pages.
func TestArtistTabsKeepHeaderPosition(t *testing.T) {
	a := catalogApp()
	a.artists[7].profile.Name = "test artist header"
	a.route = route{page: pageArtist, id: 7}
	tt := ui.NewTester(a.view, 1180, 780)
	tt.Frame()
	before, ok := tt.Find("test artist header")
	if !ok {
		t.Fatal("no artist header")
	}
	for _, tab := range []string{"专辑", "动态", "热门"} {
		if err := tt.Click(tab); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		got, ok := tt.Find("test artist header")
		if !ok || got.X != before.X || got.Y != before.Y {
			t.Fatalf("tab %s moved the header from %+v to %+v", tab, before, got)
		}
	}
}

// The playlists of 我的歌单 hide when its header is clicked, and come back
// with Enter, without changing the page or the queue.
func TestSidebarPlaylistCollapse(t *testing.T) {
	a := editTestApp()
	a.playlists[11] = &netease.Playlist{ID: 11, Name: "睡前"}
	a.pl.Cue(a.playlists[10].Tracks)
	before := a.route
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()

	if err := tt.Click("创建的歌单"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if _, ok := tt.Find("睡前"); ok {
		t.Error("the collapsed section still builds its rows")
	}
	if a.route != before || len(a.pl.Queue()) != 2 || len(a.mine) != 3 {
		t.Error("collapsing the section changed the page, queue or playlists")
	}

	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if err := tt.Click("睡前"); err != nil {
		t.Fatalf("the restored playlist cannot be opened: %v", err)
	}
	tt.Frame()
	if a.route != (route{page: pagePlaylist, id: 11}) {
		t.Errorf("opened %+v, want playlist 11", a.route)
	}
}

// 我的歌单 switches between the playlists of the site and the ones made
// here.
func TestSidebarSwitchesToBiliPlaylists(t *testing.T) {
	a := editTestApp()
	a.playlists[11] = &netease.Playlist{ID: 11, Name: "睡前"}
	a.biliPlaylists = []biliPlaylist{
		{ID: 900, Name: "从视频来的", Mode: "songs", Collected: true},
		{ID: 901, Name: "只看过的", Mode: "songs"},
	}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if _, ok := tt.Find("从视频来的"); ok {
		t.Fatal("the Bilibili playlists show before the switch")
	}
	if err := tt.Click("看哔哩哔哩歌单"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if _, ok := tt.Find("从视频来的"); !ok {
		t.Fatal("the Bilibili playlists did not show")
	}
	if _, ok := tt.Find("只看过的"); ok {
		t.Fatal("a playlist only looked at stays out of the sidebar")
	}
	if _, ok := tt.Find("睡前"); ok {
		t.Fatal("the site's playlists stayed beside them")
	}
	if err := tt.Click("看网易云歌单"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if _, ok := tt.Find("睡前"); !ok {
		t.Fatal("the site's playlists did not come back")
	}
}

// The plus still opens the naming dialog with the section collapsed, without expanding it.
func TestNewPlaylistInCollapsedSidebar(t *testing.T) {
	a := editTestApp()
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("创建的歌单"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.mineListCollapsed {
		t.Fatal("the section did not collapse")
	}
	if err := tt.Click("新建歌单"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.naming.open {
		t.Error("the plus did not open the naming dialog")
	}
	if !a.mineListCollapsed {
		t.Error("the plus also toggled the section")
	}
}

// Sidebar rows, group headings and playlist rows all start on one left edge.
func TestSidebarLeftEdgeAligned(t *testing.T) {
	a := editTestApp()
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	var left float32
	for i, label := range []string{"为你推荐", "创建的歌单", "睡前"} {
		r, ok := tt.Find(label)
		if !ok {
			t.Fatalf("no %q in the sidebar", label)
		}
		if i == 0 {
			left = r.X
			continue
		}
		if r.X != left {
			t.Errorf("%q starts at x=%g, the nav's rows at %g", label, r.X, left)
		}
	}
}
