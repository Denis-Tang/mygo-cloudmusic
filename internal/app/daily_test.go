package app

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// Recommendations stay usable while daily songs wait for their album dates.
func TestHomeOpensRecommendationsWhileDailyLoads(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.homeLoading = true
	a.recPlaylists = []netease.Playlist{{ID: 7, Name: "通勤"}}
	a.playlists[7] = &a.recPlaylists[0]
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("通勤"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.route != (route{page: pagePlaylist, id: 7}) {
		t.Fatalf("recommendation did not open: %+v", a.route)
	}
}

// The daily songs count as new from 6:00 in Beijing, wherever the listener
// is, so a home page loaded before then loads again.
func TestDailyRenewed(t *testing.T) {
	at := func(day, hour, min int) time.Time { return time.Date(2026, 10, day, hour, min, 0, 0, beijing) }
	for _, c := range []struct {
		loaded, now time.Time
		want        bool
	}{
		{at(26, 23, 0), at(27, 5, 59), false},
		{at(26, 23, 0), at(27, 6, 0), true},
		{at(27, 5, 59), at(27, 6, 1), true},
		{at(27, 6, 30), at(27, 23, 59), false},
		{at(27, 6, 30), at(28, 5, 0), false},
		{at(27, 6, 30), at(29, 1, 0), true},
	} {
		if got := dailyRenewed(c.loaded, c.now); got != c.want {
			t.Errorf("loaded %v, now %v: renewed %v", c.loaded, c.now, got)
		}
	}

	// Midnight where the listener is isn't NetEase's new day: 0:30 in
	// Los Angeles is 15:30 in Beijing, long after its 6:00.
	la, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Skip(err)
	}
	loaded := time.Date(2026, 10, 26, 23, 0, 0, 0, la)
	if dailyRenewed(loaded, time.Date(2026, 10, 27, 0, 30, 0, 0, la)) {
		t.Error("renewed at midnight in Los Angeles")
	}
	if !dailyRenewed(loaded, time.Date(2026, 10, 27, 15, 0, 0, 0, la)) {
		t.Error("not renewed past 6:00 in Beijing")
	}
}

// A portrait home page lists every recommendation the landscape one does,
// and gives the daily card more height than the flat banner.
func TestHomePortraitShowsEveryRecommendationAndATallerDailyCard(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.daily = []netease.Track{{ID: 1, Name: "日推", Duration: 200000}}
	for i := range 12 {
		a.recPlaylists = append(a.recPlaylists, netease.Playlist{ID: int64(i + 1), Name: fmt.Sprintf("歌单%02d", i+1)})
	}
	portrait := ui.NewTester(a.view, 700, 1000)
	portrait.Frame()
	if dir := os.Getenv("NM_FRAMES"); dir != "" {
		shot := func(name string) {
			f, err := os.Create(filepath.Join(dir, name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			if err := png.Encode(f, portrait.Image()); err != nil {
				t.Fatal(err)
			}
		}
		shot("home-portrait")
	}
	card, ok := portrait.Find("每日推荐卡片")
	if !ok {
		t.Fatal("no daily card in the portrait home page")
	}
	if card.H <= dailyHeight {
		t.Fatalf("the portrait daily card is %v tall, want more than %v", card.H, dailyHeight)
	}
	for i := 1; i <= 12; i++ {
		name := fmt.Sprintf("歌单%02d", i)
		if _, ok := portrait.Find(name); !ok {
			t.Errorf("portrait home page misses %q", name)
		}
	}
	landscape := ui.NewTester(a.view, 1200, 800)
	landscape.Frame()
	if dir := os.Getenv("NM_FRAMES"); dir != "" {
		f, err := os.Create(filepath.Join(dir, "home-landscape.png"))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, landscape.Image()); err != nil {
			f.Close()
			t.Fatal(err)
		}
		f.Close()
	}
	card, ok = landscape.Find("每日推荐卡片")
	if !ok {
		t.Fatal("no daily card in the landscape home page")
	}
	if card.H > dailyHeight+1 {
		t.Fatalf("the landscape daily card is %v tall, want %v", card.H, dailyHeight)
	}
}
