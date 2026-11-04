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

// Each playlist header hides only its own rows without changing the page
// or queue, and Enter restores their navigation.
func TestSidebarPlaylistCollapse(t *testing.T) {
	for _, tc := range []struct {
		name, header, row, other string
		playlist                 int64
	}{
		{"mine", "我的歌单", "睡前", "飙升榜", 11},
		{"saved", "收藏的歌单", "飙升榜", "睡前", 20},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := editTestApp()
			a.playlists[tc.playlist] = &netease.Playlist{ID: tc.playlist, Name: tc.row}
			a.pl.Cue(a.playlists[10].Tracks)
			before := a.route
			tt := ui.NewTester(a.view, 1200, 800)
			tt.Frame()

			if err := tt.Click(tc.header); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			if _, ok := tt.Find(tc.row); ok {
				t.Error("the collapsed section still builds its playlist rows")
			}
			if _, ok := tt.Find(tc.other); !ok {
				t.Error("collapsing one section hid the other section's rows")
			}
			if a.route != before || len(a.pl.Queue()) != 2 || len(a.mine) != 3 || len(a.saved) != 1 {
				t.Error("collapsing the section changed the page, queue or playlists")
			}

			tt.Key(0, ui.KeyEnter)
			tt.Frame()
			if err := tt.Click(tc.row); err != nil {
				t.Fatalf("the restored playlist cannot be opened: %v", err)
			}
			tt.Frame()
			if a.route != (route{page: pagePlaylist, id: tc.playlist}) {
				t.Errorf("opened %+v, want playlist %d", a.route, tc.playlist)
			}
		})
	}
}

// The plus still opens the naming dialog with both sections collapsed, without expanding either one.
func TestNewPlaylistInCollapsedSidebar(t *testing.T) {
	a := editTestApp()
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	for _, header := range []string{"我的歌单", "收藏的歌单"} {
		if err := tt.Click(header); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
	}
	if !a.mineCollapsed || !a.savedCollapsed {
		t.Fatal("the sections did not collapse independently")
	}
	if err := tt.Click("新建歌单"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.naming.open {
		t.Error("the plus did not open the naming dialog")
	}
	if !a.mineCollapsed || !a.savedCollapsed {
		t.Error("the plus also toggled a playlist section")
	}
}
