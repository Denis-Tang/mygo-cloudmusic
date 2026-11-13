package app

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/lyrics"
	"netease-music-mygo/internal/netease"
)

// With NM_FRAMES set, saves Now Playing at the window sizes used to compare
// it with the official client: 16:9, 3:2, 1:1 and a small window.
func TestUIParityFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	if os.Getenv("LIVE") == "" {
		t.Skip("set LIVE to fetch preview artwork from NetEase")
	}
	hits, err := netease.Search("瞳で抱いてて 石井明美", 1)
	if err != nil || len(hits) == 0 {
		t.Fatal(err)
	}
	tr := hits[0]
	a := newTestApp()
	a.settings = defaultSettings()
	for _, size := range []int{600, 120, 96} {
		img, err := fetchImage(netease.Sized(tr.Album.PicURL, size))
		if err != nil {
			t.Fatal(err)
		}
		art := &artwork{bmp: ui.NewBitmap(img), tint: dominantColor(img)}
		a.art.art[netease.Sized(tr.Album.PicURL, size)] = art
		if size == 600 {
			bd := *art
			bd.backdrop = ui.NewBitmap(blurImage(img))
			a.art.art[netease.Sized(tr.Album.PicURL, size)+"#backdrop"] = &bd
		}
	}
	a.pl.Cue([]netease.Track{tr})
	a.pl.SetLyrics(lyrics.Build("[00:00.00]作詞: 松井五郎\n[00:01.00]作曲: 林哲司\n[00:25.00]瞳で抱いてて\n[00:30.00]夜の隙間に\n[00:35.00]あなたの声が\n[00:40.00]まだ聞こえる", "", ""))
	a.pl.Seek(30 * time.Second)
	a.nowPlaying = true

	for _, s := range []struct {
		name string
		w, h int
	}{
		{"1280x720", 1280, 720},
		{"1200x800", 1200, 800},
		{"1000x1000", 1000, 1000},
		{"880x580", 880, 580},
	} {
		tt := ui.NewTester(a.view, s.w, s.h)
		tt.SetDark(true)
		tt.Advance(2 * time.Second)
		tt.Frame()
		f, _ := os.Create(filepath.Join(dir, s.name+".png"))
		if err := png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
}
