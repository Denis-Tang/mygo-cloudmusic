package app

import (
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// The window buttons show over a light page and over a dark one: their
// glyphs were white whatever the appearance, so a light window showed none
// of them, and they darkened under the pointer as the official client's do.
func TestWindowControlsAppearance(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the window buttons are Windows'")
	}
	for _, dark := range []bool{false, true} {
		a := newTestApp()
		a.settings = defaultSettings()
		tt := ui.NewTester(a.view, 1180, 780)
		tt.SetDark(dark)
		tt.Advance(300 * time.Millisecond)
		tt.Frame()
		bg := a.pal.bg
		for _, label := range []string{"最小化", "最大化", "关闭"} {
			r, ok := tt.Find(label)
			if !ok {
				t.Fatalf("dark %v: no %s button", dark, label)
			}
			rest := glyphLuminance(tt.Image(), r, bg)
			if rest < 24 {
				// White on white, or the page's color on the page's color.
				t.Fatalf("dark %v: %s glyph is invisible (luminance %.0f)", dark, label, rest)
			}
			tt.Move(r.X+r.W/2, r.Y+r.H/2)
			tt.Advance(300 * time.Millisecond)
			tt.Frame()
			hover := glyphLuminance(tt.Image(), r, bg)
			if dark && hover-rest < 8 {
				t.Errorf("%s over a dark page: hover luminance %.0f, rest %.0f; want it brighter", label, hover, rest)
			}
			if !dark && rest-hover < 8 {
				t.Errorf("%s over a light page: hover luminance %.0f, rest %.0f; want it darker", label, hover, rest)
			}
			tt.Move(600, 400)
			tt.Advance(300 * time.Millisecond)
			tt.Frame()
		}
	}
}

// glyphLuminance returns the luminance of the glyph pixel farthest from the
// page's color in the button's icon box, which the pointer leaves alone.
func glyphLuminance(img *image.RGBA, r ui.Rect, bg ui.Color) float64 {
	cx, cy := int(r.X+r.W/2), int(r.Y+r.H/2)
	var best float64
	bestDist := 0.0
	b := 0.2126*float64(bg.R) + 0.7152*float64(bg.G) + 0.0722*float64(bg.B)
	for y := cy - 7; y <= cy+7; y++ {
		for x := cx - 7; x <= cx+7; x++ {
			c := img.RGBAAt(x, y)
			l := 0.2126*float64(c.R) + 0.7152*float64(c.G) + 0.0722*float64(c.B)
			if d := math.Abs(l - b); d > bestDist {
				bestDist, best = d, l
			}
		}
	}
	return best
}

// With NM_FRAMES set, saves the main window, its bars and its window
// buttons in the light and the dark, and with the pointer over the close
// button, which darkens over a light page as the official client's does.
func TestAppearanceFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	save := func(tt *ui.Tester, name string) {
		tt.Advance(300 * time.Millisecond)
		tt.Frame()
		f, err := os.Create(filepath.Join(dir, "appearance-"+name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	for _, dark := range []bool{false, true} {
		a := newTestApp()
		a.settings = defaultSettings()
		tr := netease.Track{ID: 7, Name: "春日影", Artists: []netease.Artist{{Name: "MyGO!!!!!"}}, Duration: 199000}
		a.pl.Cue([]netease.Track{tr})
		tt := ui.NewTester(a.view, 1180, 780)
		tt.SetDark(dark)
		name := "light"
		if dark {
			name = "dark"
		}
		save(tt, name)
		r, ok := tt.Find("关闭")
		if !ok {
			t.Fatalf("%s: no close button", name)
		}
		tt.Move(r.X+r.W/2, r.Y+r.H/2)
		save(tt, name+"-hover-close")
	}
}
