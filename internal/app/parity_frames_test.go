package app

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/lyrics"
	"netease-music-mygo/internal/netease"
)

// With NM_FRAMES set, saves Now Playing at the window shapes used to compare
// it with the official client: the same synthetic cover, controls awake.
func TestNowPlayingAspectFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cover := parityCover()
	bmp := ui.NewBitmap(cover)
	soft := ui.NewBitmap(blurImage(cover))
	stage := ui.NewBitmap(stageBlur(cover))
	tint := dominantColor(cover)
	const pic = "parity-cover"
	scenes := []struct {
		name string
		w, h int
		pane int
	}{
		{"default-1180x780", 1180, 780, paneCover},
		{"min-880x580", 880, 580, paneCover},
		{"16x9-1280x720", 1280, 720, paneCover},
		{"16x9-1920x1080", 1920, 1080, paneCover},
		{"4x3-1024x768", 1024, 768, paneCover},
		{"1x1-800x800", 800, 800, paneCover},
		{"1x1-1080x1080", 1080, 1080, paneCover},
		{"9x16-720x1280", 720, 1280, paneCover},
		{"lyrics-16x9-1280x720", 1280, 720, paneLyrics},
		{"lyrics-1x1-800x800", 800, 800, paneLyrics},
	}
	for _, scene := range scenes {
		a := newTestApp()
		a.settings = defaultSettings()
		a.art.art[netease.Sized(pic, 600)+"#backdrop"] = &artwork{bmp: bmp, backdrop: soft, stage: stage, tint: tint}
		a.art.art[netease.Sized(pic, 1536)] = &artwork{bmp: bmp, tint: tint}
		a.art.art[netease.Sized(pic, 120)] = &artwork{bmp: bmp, tint: tint}
		// A size the page does not have stays empty instead of fetching.
		for range cap(a.art.sem) {
			a.art.sem <- struct{}{}
		}
		tr := netease.Track{
			ID: 42, Name: "对照曲", Duration: 200000,
			Artists: []netease.Artist{{Name: "对照歌手"}},
			Album:   netease.Album{Name: "对照专辑", PicURL: pic},
		}
		a.pl.Cue([]netease.Track{tr})
		a.pl.SetLyrics([]lyrics.Line{{At: 0, Text: "对照歌词一行", Translation: "a line for comparison"}})
		a.pl.Hold(time.Second)
		a.nowPlaying = true
		a.npPane = scene.pane
		tt := nowPlayingFrameAt(t, a, scene.w, scene.h)
		tt.Move(float32(scene.w)/2, float32(scene.h)/2)
		tt.Frame()
		f, err := os.Create(filepath.Join(dir, scene.name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, tt.Image()); err != nil {
			f.Close()
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

// parityCover is a stand-in album picture: a diagonal wash, a disc, and a
// mark in the corner, so a crop or a blur is visible without fetching art.
func parityCover() *image.RGBA {
	const n = 512
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	for y := range n {
		for x := range n {
			img.SetRGBA(x, y, color.RGBA{
				R: uint8(40 + x*180/n),
				G: uint8(30 + (n-y)*80/n),
				B: uint8(90 + y*140/n),
				A: 255,
			})
		}
	}
	cx, cy, rad := n/2, n*118/256, n*54/256
	for y := cy - rad; y <= cy+rad; y++ {
		for x := cx - rad; x <= cx+rad; x++ {
			dx, dy := x-cx, y-cy
			if dx*dx+dy*dy <= rad*rad {
				img.SetRGBA(x, y, color.RGBA{245, 236, 220, 255})
			}
		}
	}
	mark, markEnd := n*12/256, n*48/256
	for y := mark; y < markEnd; y++ {
		for x := mark; x < markEnd; x++ {
			img.SetRGBA(x, y, color.RGBA{210, 40, 50, 255})
		}
	}
	return img
}
