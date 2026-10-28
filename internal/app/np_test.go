package app

import (
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/lyrics"
	"netease-music-mygo/internal/netease"
)

func TestNowPlayingClosesAsOneLayer(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	tr := netease.Track{ID: 1, Name: "宝船", Artists: []netease.Artist{{Name: "DOUDOU"}}, Duration: 200000}
	a.pl.Cue([]netease.Track{tr})
	a.pl.SetLyrics(nil)
	tt := ui.NewTester(a.view, 1200, 800)
	tt.SetDark(false)
	a.nowPlaying = true
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	if !tt.HasText("待播清单") {
		t.Fatal("now playing did not open")
	}
	// Mid-way out, it still covers the page and the sidebar alike.
	a.nowPlaying = false
	tt.Frame()
	tt.Advance(60 * time.Millisecond)
	tt.Frame()
	img := tt.Image()
	sx, sy := img.Bounds().Dx(), img.Bounds().Dy()
	dark := func(x, y float32) bool {
		r, g, b, _ := img.At(int(x*float32(sx)/1200), int(y*float32(sy)/800)).RGBA()
		return (r+g+b)/3 < 0x9000
	}
	if !dark(900, 400) || !dark(100, 400) {
		t.Errorf("page %v, sidebar %v: the closing layer should cover both", dark(900, 400), dark(100, 400))
	}
	tt.Advance(2 * time.Second)
	tt.Frame()
	if tt.HasText("待播清单") {
		t.Error("now playing still shows after closing")
	}
}

func TestQueueStaysAsATrackIsPlayed(t *testing.T) {
	a := newTestApp()
	var tracks []netease.Track
	for i := range 12 {
		tracks = append(tracks, netease.Track{ID: int64(i + 1), Name: fmt.Sprintf("歌%02d", i), Duration: 200000})
	}
	a.pl.Cue(tracks)
	a.nowPlaying, a.npPane = true, paneQueue
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	tt.Frame()
	if err := tt.Click("歌05"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if got := a.pl.Current().Name; got != "歌05" {
		t.Fatalf("playing %q, want 歌05", got)
	}
	// The tracks before it are still listed, as they were.
	for _, s := range []string{"歌01", "歌04", "歌06", "歌09"} {
		if !tt.HasText(s) {
			t.Errorf("missing %q in %q", s, tt.Texts())
		}
	}
}

// The app chooses Now Playing's pane before it makes the player: it
// crashed on start when the choice asked the player what played.
func TestLyricsPaneBeforePlayer(t *testing.T) {
	a := &app{}
	if pane := a.lyricsPaneChosen(); pane != paneCover {
		t.Errorf("pane %d", pane)
	}
}

// 上一首 plays the song before, however far this one has played: a few
// seconds in, it went back to this one's start, and took a second click.
func TestPrevGoesBackASong(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.pl.Cue([]netease.Track{{ID: 1, Name: "一", Duration: 200000}, {ID: 2, Name: "二", Duration: 200000}})
	a.pl.SetLyrics(nil)
	a.pl.Hold(30 * time.Second)
	a.nowPlaying = true
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	if err := tt.Click("上一首"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if id := a.pl.Current().ID; id != 2 {
		t.Errorf("playing %d, want the one before, 2", id)
	}
}

// Now Playing holds still as the quality's badge goes, while the next song
// loads, and comes back: the row of times grew with it, moving the column.
func TestNowPlayingHoldsStillWithoutBadge(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.pl.Cue([]netease.Track{{ID: 1, Name: "一", Duration: 200000}})
	a.pl.SetLyrics(nil)
	a.nowPlaying, a.npPane = true, paneCover
	tt := ui.NewTester(a.view, 1200, 800)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tt.Frame()
	var at []float32
	for _, st := range []*netease.Stream{{Level: "lossless"}, nil} {
		a.pl.SetStream(st)
		tt.Frame()
		tt.Advance(time.Second)
		tt.Frame()
		r, ok := tt.Find("播放进度")
		if !ok {
			t.Fatal("no progress over the playing page")
		}
		at = append(at, r.Y)
	}
	if at[0] != at[1] {
		t.Errorf("the progress moved from %v to %v as the badge went", at[0], at[1])
	}
}

// The buttons over the top of Now Playing choose its pane: a click on
// one opens it, and it goes back to the cover.
func TestPaneButtonsChoosePane(t *testing.T) {
	for _, pane := range []int{paneLyrics, paneQueue, paneComments} {
		a := newTestApp()
		a.settings = defaultSettings()
		a.pl.Cue([]netease.Track{{ID: 1, Name: "宝船", Duration: 200000}})
		a.pl.SetLyrics(nil)
		a.nowPlaying = true
		tt := ui.NewTester(a.view, 1200, 800)
		tt.Frame()
		tt.Move(600, 400) // over the picture: the bar of it rises
		tt.Advance(400 * time.Millisecond)
		tt.Frame()
		r, ok := tt.Find(paneButtons[pane])
		if !ok {
			t.Fatalf("no %s button", paneButtons[pane])
		}
		// The buttons show while the pointer is in the window: over one,
		// they fade in, and once in, a click on it lands.
		tt.Move(r.X+r.W/2, r.Y+r.H/2)
		tt.Advance(400 * time.Millisecond)
		tt.Frame()
		if err := tt.Click(paneButtons[pane]); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if a.npPane != pane {
			t.Errorf("the %s button chose pane %d, want %d", paneButtons[pane], a.npPane, pane)
		}
	}
}

// paneButtons name the top strip's buttons that open their panes, by
// accessibility name, which the texts on them alone would share.
var paneButtons = map[int]string{
	paneLyrics:   "歌词面板",
	paneQueue:    "待播清单面板",
	paneComments: "评论面板",
}

// Now Playing over a song at its start shows its name and singer without
// a line: reading the lyrics as the line before the first was there
// panicked, and opening the page over a song at 0:00 closed the app.
func TestNowPlayingBeforeTheFirstLine(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.pl.Cue([]netease.Track{{ID: 1, Name: "宝船", Duration: 200000}})
	a.pl.SetLyrics([]lyrics.Line{{At: 5 * time.Second, Text: "第一句"}})
	a.pl.Hold(0)
	a.nowPlaying = true
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if _, ok := tt.Find("宝船"); !ok {
		t.Fatal("the song's name is gone")
	}
	if _, ok := tt.Find("第一句"); ok {
		t.Error("a line shows before the song reaches it")
	}
	a.pl.Hold(5 * time.Second)
	tt.Frame()
	if _, ok := tt.Find("第一句"); !ok {
		t.Errorf("the first line does not show once reached: %q", tt.Texts())
	}
}

// The playing page's controls go once the listener rests, and come back
// when the pointer moves: the page drawing what plays is not the listener
// acting, which once woke it on every frame and kept it from ever resting.
func TestNowPlayingHidesItsControlsWhenTheListenerRests(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.pl.Cue([]netease.Track{{ID: 1, Name: "宝船", Duration: 200000}})
	a.pl.SetLyrics(nil)
	a.nowPlaying = true
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if _, ok := tt.Find("返回"); !ok {
		t.Fatal("no way back over the picture")
	}
	if _, ok := tt.Find("歌词"); !ok {
		t.Fatal("no controls over the picture")
	}
	tt.Advance(npIdle + 200*time.Millisecond)
	tt.Frame()
	if d := tt.Now().Sub(a.npActiveAt); d < npIdle {
		t.Errorf("the page woke %v ago, want it resting", d)
	}
	if _, ok := tt.Find("返回"); ok {
		t.Error("the way back stayed once the listener rested")
	}
	if _, ok := tt.Find("歌词"); ok {
		t.Error("the controls stayed once the listener rested")
	}
	tt.Move(600, 400)
	tt.Frame()
	if _, ok := tt.Find("返回"); !ok {
		t.Error("the way back did not come with the pointer")
	}
	if _, ok := tt.Find("歌词"); !ok {
		t.Error("the controls did not come with the pointer")
	}
}
