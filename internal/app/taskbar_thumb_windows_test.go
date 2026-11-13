//go:build windows

package app

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"syscall"
	"testing"
	"unsafe"

	"github.com/egoist/mygo"

	"netease-music-mygo/internal/netease"
)

// Windows sends 0x0326 for Peek; listening on 0x0325 left its request
// unanswered, so the taskbar showed a blank page instead of the capture.
func TestTaskbarLivePreviewMessage(t *testing.T) {
	if wmSendIconicLivePreview != 0x0326 {
		t.Fatalf("live preview message = %#x, want WM_DWMSENDICONICLIVEPREVIEWBITMAP (0x0326)", wmSendIconicLivePreview)
	}
}

// Captured PNG pixels reach DWM as premultiplied BGRA; multiplying their
// alpha twice used to darken translucent content.
func TestTaskbarCapturedBitmap(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	src.SetNRGBA(0, 0, color.NRGBA{200, 100, 50, 128})
	src.SetNRGBA(1, 0, color.NRGBA{20, 90, 180, 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(&buf)
	if err != nil {
		t.Fatal(err)
	}
	hbm := taskbarHBitmap(img)
	if hbm == 0 {
		t.Fatal("no captured bitmap")
	}
	defer procDeleteObject.Call(hbm)
	var bm taskbarBitmap
	n, _, _ := taskbarGdi32.NewProc("GetObjectW").Call(hbm, unsafe.Sizeof(bm), uintptr(unsafe.Pointer(&bm)))
	if n == 0 || bm.Bits == 0 {
		t.Fatal("no captured pixels")
	}
	want := []byte{25, 50, 100, 128, 180, 90, 20, 255}
	got := make([]byte, len(want))
	syscall.NewLazyDLL("kernel32.dll").NewProc("RtlMoveMemory").Call(uintptr(unsafe.Pointer(&got[0])), bm.Bits, uintptr(len(got)))
	if !bytes.Equal(got, want) {
		t.Errorf("captured BGRA = %v, want %v", got, want)
	}
}

// Peek used to expose the desktop through the native sidebar's transparent
// pixels; its cached page must be opaque, including translucent content.
func TestTaskbarPreviewOpaque(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 3, 1))
	src.SetNRGBA(1, 0, color.NRGBA{200, 100, 50, 128})
	src.SetNRGBA(2, 0, color.NRGBA{20, 90, 180, 255})
	bg := color.RGBA{20, 20, 22, 255}
	page := taskbarPreview(src, bg)
	if got := page.RGBAAt(0, 0); got != bg {
		t.Errorf("transparent sidebar = %v, want %v", got, bg)
	}
	if got := page.RGBAAt(2, 0); got != (color.RGBA{20, 90, 180, 255}) {
		t.Errorf("opaque content = %v", got)
	}
	for x := range 3 {
		if got := page.RGBAAt(x, 0); got.A != 255 {
			t.Errorf("pixel %d alpha = %d, want 255", x, got.A)
		}
	}
}

// The card shows the cover whole and centred on the card's own colour, and the
// note stands in while there is no cover.
func TestTaskbarCard(t *testing.T) {
	bg := color.RGBA{0x14, 0x14, 0x16, 0xff}
	cover := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for i := range cover.Pix {
		cover.Pix[i] = 0xff
	}
	for i := 0; i < len(cover.Pix); i += 4 {
		cover.Pix[i+0], cover.Pix[i+1], cover.Pix[i+2] = 0x20, 0x90, 0x40
	}
	card := taskbarCard(200, 150, cover, bg)
	if got := card.RGBAAt(100, 75); got.R < 0x10 || got.G < 0x80 {
		t.Errorf("the middle of the card is %v, want the cover", got)
	}
	// The cover fills the whole picture area, as the official card shows it,
	// so no band of the card's own colour shows beside it.
	if got := card.RGBAAt(2, 2); got.R < 0x10 || got.G < 0x80 {
		t.Errorf("the card's corner is %v, want the cover", got)
	}
	note := taskbarCard(200, 150, nil, bg)
	gold := 0
	for y := range 150 {
		for x := range 200 {
			c := note.RGBAAt(x, y)
			if c.R > 0xd0 && c.G > 0xb0 && c.B < 0xc0 && c.A == 0xff {
				gold++
			}
		}
	}
	if gold < 50 {
		t.Errorf("the note covers %d pixels, want a visible note", gold)
	}
	if got := note.RGBAAt(2, 2); got != bg {
		t.Errorf("beside the note the card is %v, want its background %v", got, bg)
	}
}

// The title is the name of the track that plays; the app's own name stands in
// while nothing plays.
func TestTaskbarTitle(t *testing.T) {
	if got := taskbarTitle(&netease.Track{Name: "我不难过"}); got != "我不难过" {
		t.Errorf("title is %q, want the track's name", got)
	}
	for _, tr := range []*netease.Track{nil, {}, {Name: "  "}} {
		if got := taskbarTitle(tr); got != mygo.App.Name() {
			t.Errorf("title is %q, want the app's name %q", got, mygo.App.Name())
		}
	}
}

// The bitmap handed to DWM is the size of the card: a header of the wrong
// length made CreateDIBSection fail and the card never appeared.
func TestTaskbarHBitmap(t *testing.T) {
	card := taskbarCard(120, 90, nil, color.RGBA{0x14, 0x14, 0x16, 0xff})
	hbm := taskbarHBitmap(card)
	if hbm == 0 {
		t.Fatal("no bitmap")
	}
	getObject := taskbarGdi32.NewProc("GetObjectW")
	var bm taskbarBitmap
	n, _, _ := getObject.Call(hbm, unsafe.Sizeof(bm), uintptr(unsafe.Pointer(&bm)))
	procDeleteObject.Call(hbm)
	if n == 0 {
		t.Fatal("the bitmap has no header")
	}
	if bm.Width != 120 || bm.Height != 90 {
		t.Errorf("the bitmap is %dx%d, want 120x90", bm.Width, bm.Height)
	}
}

// The bitmap handed to DWM is square, so the card's picture is square and the
// cover fills it instead of leaving wide margins beside it.
func TestTaskbarCardSize(t *testing.T) {
	for _, c := range []struct{ maxW, maxH, w, h int }{
		{200, 128, 128, 128},
		{200, 150, 150, 150},
		{128, 128, 128, 128},
		{0, 0, 0, 0},
	} {
		w, h := taskbarCardSize(c.maxW, c.maxH)
		if w != c.w || h != c.h {
			t.Errorf("asked %dx%d, draw %dx%d, want %dx%d", c.maxW, c.maxH, w, h, c.w, c.h)
		}
	}
}

// WM_DWMSENDICONICTHUMBNAIL names the widest size in the high word of lParam
// and the tallest in the low word; read the other way, the card is drawn to
// the wrong shape.
func TestTaskbarAskedSize(t *testing.T) {
	w, h := taskbarAskedSize(uintptr(200)<<16 | 150)
	if w != 200 || h != 150 {
		t.Errorf("asked %dx%d, want 200x150", w, h)
	}
}

// A cover fills the picture area, cropped, as the official card shows it: no
// band of the card's own colour shows beside it.
func TestTaskbarCardFillsThePicture(t *testing.T) {
	wide := image.NewRGBA(image.Rect(0, 0, 200, 100))
	for i := 0; i < len(wide.Pix); i += 4 {
		wide.Pix[i+0], wide.Pix[i+1], wide.Pix[i+2], wide.Pix[i+3] = 0xf0, 0x30, 0x30, 0xff
	}
	bg := color.RGBA{0x2d, 0x2d, 0x2f, 0xff}
	card := taskbarCard(128, 128, wide, bg)
	for _, p := range [][2]int{{2, 2}, {64, 64}, {125, 125}} {
		if got := card.RGBAAt(p[0], p[1]); got.R < 0xc0 || got.G > 0x60 {
			t.Errorf("the card at %v is %v, want the cover", p, got)
		}
	}
}
