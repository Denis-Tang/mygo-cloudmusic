//go:build windows

package app

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"strings"
	"syscall"
	"unsafe"

	"github.com/egoist/mygo"
	"golang.org/x/image/draw"

	"netease-music-mygo/internal/netease"
)

// The window's preview card, which Windows draws over the taskbar button when
// the pointer rests there. Asking for an iconic thumbnail replaces the live
// picture of the window with the cover of the track that plays, as the
// official client shows it; the card's header keeps the window title, which
// follows the track too.

var (
	taskbarDwmapi = syscall.NewLazyDLL("dwmapi.dll")

	procDwmSetWindowAttribute      = taskbarDwmapi.NewProc("DwmSetWindowAttribute")
	procDwmSetIconicThumbnail      = taskbarDwmapi.NewProc("DwmSetIconicThumbnail")
	procDwmSetIconicLivePreview    = taskbarDwmapi.NewProc("DwmSetIconicLivePreviewBitmap")
	procDwmInvalidateIconicBitmaps = taskbarDwmapi.NewProc("DwmInvalidateIconicBitmaps")
	procGetDC                      = taskbarUser32.NewProc("GetDC")
	procReleaseDC                  = taskbarUser32.NewProc("ReleaseDC")
	procCreateCompatibleDC         = taskbarGdi32.NewProc("CreateCompatibleDC")
	procDeleteDC                   = taskbarGdi32.NewProc("DeleteDC")
	procCreateDIBSection           = taskbarGdi32.NewProc("CreateDIBSection")
)

// taskbarCardBackground is the card's own colour, measured off the official
// card's picture area: a neutral dark that Explorer's frame blends with.
var taskbarCardBackground = color.RGBA{0x2d, 0x2d, 0x2f, 0xff}

const (
	// DWMWA_FORCE_ICONIC_REPRESENTATION: Windows asks the window for a
	// bitmap instead of scaling a live copy of it.
	thumbnailForceIconic = 7
	thumbnailHasIconic   = 10
	// WM_DWMSENDICONICTHUMBNAIL: the high word of lParam is the maximum
	// width, and the low word is the maximum height.
	wmSendIconicThumbnail = 0x0323
	// WM_DWMSENDICONICLIVEPREVIEWBITMAP asks for the window itself, which
	// Windows shows while the pointer rests on the preview.
	wmSendIconicLivePreview = 0x0326
)

// taskbarThumbnail holds what the card shows and the picture of the cover
// once it has loaded.
type taskbarThumbnail struct {
	hwnd    uintptr
	url     string      // the cover the image below is of
	cover   image.Image // nil until it loads, or while nothing plays
	maxW    int         // the largest card DWM asked for, until it asks
	maxH    int
	failed  bool
	preview uintptr // opaque page bitmap, ready before Peek asks for it
}

// attach asks DWM for a bitmap of the window instead of a live copy. A DWM
// too old to know the attributes leaves the live preview.
func (t *taskbarThumbnail) attach(hwnd uintptr) {
	t.hwnd = hwnd
	one := int32(1)
	hr, _, _ := procDwmSetWindowAttribute.Call(hwnd, thumbnailForceIconic, uintptr(unsafe.Pointer(&one)), unsafe.Sizeof(one))
	if taskbarFailed(hr) {
		t.failed = true
		return
	}
	// Without this one, DwmSetIconicThumbnail answers E_INVALIDARG.
	hr, _, _ = procDwmSetWindowAttribute.Call(hwnd, thumbnailHasIconic, uintptr(unsafe.Pointer(&one)), unsafe.Sizeof(one))
}

// refresh draws the card again once DWM has said how big it may be. Until it
// asks, there is no preview to fill; asking again every frame would draw
// bitmaps DWM refuses, which is what a first size guess did.
func (t *taskbarThumbnail) refresh() {
	if t.hwnd == 0 || t.failed || t.maxW == 0 {
		return
	}
	if w, h := taskbarCardSize(t.maxW, t.maxH); w > 0 {
		procDwmInvalidateIconicBitmaps.Call(t.hwnd)
		t.set(w, h)
		t.live()
	}
}

// set answers DWM without invalidating the live preview it already holds.
func (t *taskbarThumbnail) set(w, h int) bool {
	hbmp := taskbarHBitmap(taskbarCard(w, h, t.cover, taskbarCardBackground))
	if hbmp == 0 {
		return false
	}
	hr, _, _ := procDwmSetIconicThumbnail.Call(t.hwnd, hbmp, 0)
	procDeleteObject.Call(hbmp)
	if taskbarFailed(hr) {
		return false
	}
	return true
}

// taskbarAskedSize reads the size WM_DWMSENDICONICTHUMBNAIL asks for: the high
// word of lParam is the widest the card may be, the low word the tallest.
func taskbarAskedSize(lParam uintptr) (int, int) {
	return int((lParam >> 16) & 0xffff), int(lParam & 0xffff)
}

// taskbarTitle is the window's title: the name of the track that plays, and
// the app's own name while nothing does.
func taskbarTitle(tr *netease.Track) string {
	if tr != nil {
		if name := strings.TrimSpace(tr.Name); name != "" {
			return name
		}
	}
	return mygo.App.Name()
}

// show follows the track: the cover loads in the background, and DWM makes
// the card again once it has.
func (t *taskbarThumbnail) show(tr *netease.Track) {
	url := ""
	if tr != nil {
		url = netease.Sized(tr.Album.PicURL, 600)
	}
	if url == t.url {
		return
	}
	t.url, t.cover = url, nil
	t.refresh()
	if url == "" || t.failed {
		return
	}
	go func() {
		img, err := fetchImage(url)
		post := func(fn func()) {
			if a := taskbarApp; a != nil {
				a.post(fn)
			}
		}
		post(func() {
			if url != t.url {
				return
			}
			if err == nil {
				t.cover = img
			}
			t.refresh()
		})
	}()
}

// reply draws the card at the size DWM asked for: the high word of lParam is
// the widest it takes, the low word the tallest.
func (t *taskbarThumbnail) reply(lParam uintptr) {
	if t.hwnd == 0 || t.failed {
		return
	}
	w, h := taskbarAskedSize(lParam)
	if w <= 0 || h <= 0 {
		return
	}
	t.maxW, t.maxH = min(w, 1024), min(h, 1024)
	w, h = taskbarCardSize(t.maxW, t.maxH)
	if w <= 0 || h <= 0 {
		return
	}
	t.set(w, h)
}

// capture prepares the page outside DWM's synchronous request. Waiting to
// render and encode a page inside that request left Peek white at first.
func (t *taskbarThumbnail) capture(win *mygo.Window, bg color.RGBA) {
	go func() {
		data, err := win.CapturePage()
		if err != nil {
			return
		}
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			return
		}
		hbmp := taskbarHBitmap(taskbarPreview(img, bg))
		if hbmp == 0 {
			return
		}
		win.Update(func() {
			if t.hwnd == 0 {
				procDeleteObject.Call(hbmp)
				return
			}
			if t.preview != 0 {
				procDeleteObject.Call(t.preview)
			}
			t.preview = hbmp
			t.live()
		})
	}()
}

// taskbarPreview replaces native vibrancy's transparent areas with the
// page's background; Peek has no material layer behind the captured pixels.
func taskbarPreview(img image.Image, bg color.RGBA) *image.RGBA {
	page := image.NewRGBA(image.Rect(0, 0, img.Bounds().Dx(), img.Bounds().Dy()))
	draw.Draw(page, page.Bounds(), image.NewUniform(bg), image.Point{}, draw.Src)
	draw.Draw(page, page.Bounds(), img, img.Bounds().Min, draw.Over)
	return page
}

// live answers Peek immediately with the page captured ahead of its request.
func (t *taskbarThumbnail) live() {
	if t.hwnd == 0 || t.preview == 0 {
		return
	}
	procDwmSetIconicLivePreview.Call(t.hwnd, t.preview, 0, 0)
}

// taskbarCardSize is the bitmap to hand DWM for the size it asked for: a
// square, so the card's picture fits the cover, inside the widest and tallest
// DWM accepts. The official card has no band of its own colour beside the
// cover, and neither does this one.
func taskbarCardSize(maxW, maxH int) (int, int) {
	if maxW <= 0 || maxH <= 0 {
		return 0, 0
	}
	side := min(maxW, maxH)
	return side, side
}

// taskbarCard draws the card: the cover over the whole picture area, cropped
// to its shape as the official card shows it, and the app's note on the
// card's own colour while there is no cover.
func taskbarCard(w, h int, cover image.Image, bg color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = bg.R, bg.G, bg.B, 0xff
	}
	if cover == nil {
		note := min(w, h) * 2 / 5
		taskbarNote(img, (w-note)/2, (h-note)/2, note)
		return img
	}
	b := cover.Bounds()
	// The cover fills the picture area; the longer side is cropped, centred.
	if b.Dx()*h > b.Dy()*w {
		crop := b.Dy() * w / h
		b.Min.X += (b.Dx() - crop) / 2
		b.Max.X = b.Min.X + crop
	} else if b.Dy()*w > b.Dx()*h {
		crop := b.Dx() * h / w
		b.Min.Y += (b.Dy() - crop) / 2
		b.Max.Y = b.Min.Y + crop
	}
	draw.CatmullRom.Scale(img, image.Rect(0, 0, w, h), cover, b, draw.Src, nil)
	return img
}

// taskbarNote draws the app's note in its gold on a square side across, as the
// icon draws it: two rings under a beam.
func taskbarNote(img *image.RGBA, x0, y0, side int) {
	gold := color.RGBA{0xf3, 0xd9, 0x9b, 0xff}
	scale := float64(side) / 24
	inside := func(x, y float64) bool {
		if math.Hypot(x-6, y-18) < 3 || math.Hypot(x-18, y-16) < 3 {
			return true
		}
		return taskbarSegment(x, y, 9, 18, 9, 5) < 1 ||
			taskbarSegment(x, y, 9, 5, 21, 3) < 1 ||
			taskbarSegment(x, y, 21, 3, 21, 16) < 1
	}
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			hits := 0
			for sy := range 4 {
				for sx := range 4 {
					px := (float64(x) + (float64(sx)+0.5)/4) / scale
					py := (float64(y) + (float64(sy)+0.5)/4) / scale
					if inside(px, py) {
						hits++
					}
				}
			}
			if hits == 0 {
				continue
			}
			img.SetRGBA(x0+x, y0+y, color.RGBA{gold.R, gold.G, gold.B, uint8(255 * hits / 16)})
		}
	}
}

// The DIB DWM takes: a 32 bit device independent bitmap, top down.
type taskbarBitmapInfoHeader struct {
	Size, Width, Height    int32
	Planes, BitCount       int16
	Compression, SizeImage int32
	XPels, YPels           int32
	ClrUsed, ClrImportant  int32
}

type taskbarBitmapInfo struct {
	Header taskbarBitmapInfoHeader
	Colors [3]uint32
}

// taskbarDIB makes a top down 32 bit DIB section of w by h, its pixels at
// bits, as DWM reads bitmaps.
func taskbarDIB(w, h int) (uintptr, uintptr) {
	screen, _, _ := procGetDC.Call(0)
	if screen == 0 {
		return 0, 0
	}
	dc, _, _ := procCreateCompatibleDC.Call(screen)
	procReleaseDC.Call(0, screen)
	if dc == 0 {
		return 0, 0
	}
	defer procDeleteDC.Call(dc)
	var bi taskbarBitmapInfo
	bi.Header.Size = int32(unsafe.Sizeof(bi.Header))
	bi.Header.Width = int32(w)
	bi.Header.Height = int32(-h)
	bi.Header.Planes = 1
	bi.Header.BitCount = 32
	var bits uintptr
	hbmp, _, _ := procCreateDIBSection.Call(dc, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if hbmp == 0 || bits == 0 {
		return 0, 0
	}
	return hbmp, bits
}

// taskbarHBitmap copies img into a DIB section, which DWM reads as
// premultiplied BGRA.
func taskbarHBitmap(img image.Image) uintptr {
	if img == nil || img.Bounds().Empty() {
		return 0
	}
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	hbmp, bits := taskbarDIB(w, h)
	if hbmp == 0 {
		return 0
	}
	rgba := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(rgba, rgba.Bounds(), img, img.Bounds().Min, draw.Src)
	out := unsafe.Slice((*byte)(unsafe.Pointer(bits)), w*h*4)
	for i := 0; i < len(out); i += 4 {
		// RGBA is already premultiplied; DWM only needs the channels swapped.
		out[i+0], out[i+1], out[i+2], out[i+3] = rgba.Pix[i+2], rgba.Pix[i+1], rgba.Pix[i], rgba.Pix[i+3]
	}
	return hbmp
}
