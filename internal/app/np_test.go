package app

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/lyrics"
	"netease-music-mygo/internal/netease"
)

// The full-page cover used to stay blank until its separate high-resolution request finished.
func TestFullCoverShowsWhileHighResolutionLoads(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	tr := netease.Track{ID: 1, Name: "宝船", Album: netease.Album{PicURL: "cover"}}
	a.pl.Cue([]netease.Track{tr})
	a.nowPlaying = true
	// Keep the high-resolution request pending without using the network.
	for range cap(a.art.sem) {
		a.art.sem <- struct{}{}
	}
	red := ui.NewBitmap(&image.RGBA{Pix: []byte{220, 0, 0, 255}, Stride: 4, Rect: image.Rect(0, 0, 1, 1)})
	a.art.art[netease.Sized("cover", 600)+"#backdrop"] = &artwork{bmp: red}
	tt := nowPlayingFrame(t, a)
	r, g, _, _ := tt.Image().At(200, 200).RGBA()
	if r <= g || r < 0x4000 {
		t.Fatal("ordinary cover did not fill the playing page while high resolution was pending")
	}
	green := ui.NewBitmap(&image.RGBA{Pix: []byte{0, 220, 0, 255}, Stride: 4, Rect: image.Rect(0, 0, 1, 1)})
	a.art.art[netease.Sized("cover", 1536)] = &artwork{bmp: green}
	tt.Frame()
	r, g, _, _ = tt.Image().At(200, 200).RGBA()
	if g <= r || g < 0x4000 {
		t.Fatal("high-resolution cover did not replace the ordinary cover")
	}
}

// The more panel folds each list, resizes at its divider, moves, and closes on an outside click.
func TestDrawerCustomize(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.pl.Cue([]netease.Track{{ID: 1, Name: "宝船", Duration: 200000}})
	a.nowPlaying, a.npPane = true, paneQueue
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	shot := func(name string) {
		if dir := os.Getenv("NM_FRAMES"); dir != "" {
			f, err := os.Create(filepath.Join(dir, "drawer-"+name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			png.Encode(f, tt.Image())
		}
	}
	shot("expanded")
	for _, name := range []string{"待播清单", "评论"} {
		if err := tt.Click(name); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
	}
	if a.npDrawer.collapsed != [2]bool{true, true} {
		t.Fatalf("collapsed = %v", a.npDrawer.collapsed)
	}
	tt.Advance(350 * time.Millisecond)
	tt.Frame()
	shot("collapsed")
	for _, name := range []string{"待播清单", "评论"} {
		tt.Click(name)
		tt.Frame()
	}
	tt.Advance(350 * time.Millisecond)
	tt.Frame()
	divider, ok := tt.Find("Divider")
	if !ok {
		t.Fatalf("no divider: %q", tt.Texts())
	}
	before := a.npDrawer.share
	x, y := divider.X+divider.W/2, divider.Y+divider.H/2
	tt.Press(x, y)
	tt.Move(x, y-55)
	tt.Release(x, y-55)
	tt.Frame()
	if a.npDrawer.share >= before || a.npDrawer.share <= 0 {
		t.Fatalf("divider moved share from %v to %v", before, a.npDrawer.share)
	}
	grip, ok := tt.Find("移动更多面板")
	if !ok {
		t.Fatal("no panel grip")
	}
	x, y = grip.X+grip.W/2, grip.Y+grip.H/2
	tt.Press(x, y)
	tt.Move(x+160, y-100)
	tt.Release(x+160, y-100)
	tt.Advance(time.Second)
	tt.Frame()
	if a.npDrawer.position.X < 150 || a.npDrawer.position.Y > -90 {
		t.Fatalf("panel position = %+v", a.npDrawer.position)
	}
	shot("moved")
	grip, _ = tt.Find("移动更多面板")
	x, y = grip.X+grip.W/2, grip.Y+grip.H/2
	tt.Press(x, y)
	tt.Move(x+3000, y-2000)
	tt.Release(x+3000, y-2000)
	tt.Advance(time.Second)
	tt.Frame()
	panel, _ := tt.Find("待播清单与评论")
	if panel.X < 12 || panel.Y < 8 || panel.X+panel.W > 1188 || panel.Y+panel.H > 792 {
		t.Fatalf("panel escaped the window: %+v", panel)
	}
	tt.ClickAt(100, 300)
	tt.Frame()
	if a.npDrawer.open || a.npPane != paneCover {
		t.Fatalf("outside click left drawer open on pane %d", a.npPane)
	}
}

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
	if !tt.HasText("更多") {
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
	if tt.HasText("更多") {
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
	a.nowPlaying = true
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	tt.Frame()
	tt.Move(600, 400) // the bar rises with the pointer
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	if err := tt.Click("更多"); err != nil {
		t.Fatalf("the [...] opening the queue: %v", err)
	}
	for range 3 {
		tt.Frame()
		tt.Advance(100 * time.Millisecond)
	}
	// The queue's rows, down the top list of the panel: one of them plays.
	row, ok := tt.Find("歌02")
	if !ok || row.H <= 0 {
		t.Fatalf("no row of the queue in the panel, over %q", tt.Texts())
	}
	tt.Press(600, row.Y+row.H/2)
	tt.Release(600, row.Y+row.H/2)
	tt.Frame()
	tt.Frame()
	if got := a.pl.Current().Name; got != "歌02" {
		t.Fatalf("playing %q, want 歌02", got)
	}
	// The tracks before it are still listed, as they were.
	for _, s := range []string{"歌01", "歌03", "歌04"} {
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

// The [...] of the playing page's bar opens the queue and the comments
// over the middle of the page, and a click on it again closes them, as a
// press outside them or the escape key does.
func TestDrawerOpensAndCloses(t *testing.T) {
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
	tt.Move(1000, 756)
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	if err := tt.Click("更多"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	if !a.npDrawer.open || a.npPane != paneQueue {
		t.Fatalf("the [...] left the panel %v, on pane %d", a.npDrawer.open, a.npPane)
	}
	// It opens over the middle of the page, its foot towards the button.
	q, ok := tt.Find("待播清单")
	if !ok {
		t.Fatalf("no queue in the panel, over %q", tt.Texts())
	}
	if q.X < 300 || q.X+q.W > 900 {
		t.Errorf("the queue's head at %.0f..%.0f: the panel should open over the middle", q.X, q.X+q.W)
	}
	// A click on it again closes it.
	tt.Move(1000, 756)
	tt.Advance(50 * time.Millisecond)
	if err := tt.Click("更多"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.npDrawer.open || a.npPane != paneCover {
		t.Errorf("a second click left the panel %v, on pane %d", a.npDrawer.open, a.npPane)
	}
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

// Resting, the playing page hides the pointer with its controls, as a
// video does; moving it brings both back.
func TestNowPlayingHidesThePointerWhenItRests(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.pl.Cue([]netease.Track{{ID: 1, Name: "宝船", Duration: 200000}})
	a.pl.SetLyrics(nil)
	a.nowPlaying = true
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	tt.Move(600, 300) // over the picture
	tt.Frame()
	if got := tt.Cursor(); got != ui.CursorDefault {
		t.Fatalf("pointer %v over the picture, want the default", got)
	}
	tt.Advance(npIdle + 300*time.Millisecond)
	tt.Frame()
	if got := tt.Cursor(); got != ui.CursorNone {
		t.Errorf("pointer %v once the listener rested, want it hidden", got)
	}
	tt.Move(700, 320)
	tt.Frame()
	if got := tt.Cursor(); got != ui.CursorDefault {
		t.Errorf("pointer %v after the move, want the default", got)
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
	if _, ok := tt.Find("更多"); !ok {
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
	if _, ok := tt.Find("更多"); ok {
		t.Error("the controls stayed once the listener rested")
	}
	tt.Move(600, 400)
	// The move itself asks for the frame that brings them back: they must
	// not wait for another act.
	tt.Advance(50 * time.Millisecond)
	if _, ok := tt.Find("返回"); !ok {
		t.Error("the way back did not come with the pointer")
	}
	if _, ok := tt.Find("更多"); !ok {
		t.Error("the controls did not come with the pointer")
	}
}
