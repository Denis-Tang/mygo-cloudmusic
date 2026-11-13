//go:build windows

package app

import (
	"bytes"
	"image/color"
	"image/png"
	"testing"
	"unsafe"
)

// taskbarBitmap is GDI's BITMAP, which GetObject fills for an icon's color
// bitmap.
type taskbarBitmap struct {
	Type       int32
	Width      int32
	Height     int32
	WidthBytes int32
	Planes     uint16
	BitsPixel  uint16
	Bits       uintptr
}

// The taskbar draws a thumbnail toolbar's icons at 16 by 16; a larger
// color bitmap was added without error but drew as an empty button.
func TestTaskbarIconsAreTheTaskbarsSize(t *testing.T) {
	getObject := taskbarGdi32.NewProc("GetObjectW")
	getIconInfo := taskbarUser32.NewProc("GetIconInfo")
	for _, shape := range []int{taskbarShapePrev, taskbarShapePlay, taskbarShapePause, taskbarShapeNext, taskbarShapeHeart, taskbarShapeHeartOn} {
		icon := taskbarIcon(shape, 32)
		if icon == 0 {
			t.Fatalf("shape %d made no icon", shape)
		}
		var info taskbarIconInfo
		ok, _, _ := getIconInfo.Call(icon, uintptr(unsafe.Pointer(&info)))
		if ok == 0 {
			procDestroyIcon.Call(icon)
			t.Fatalf("shape %d is not an icon", shape)
		}
		var bm taskbarBitmap
		n, _, _ := getObject.Call(info.HbmColor, unsafe.Sizeof(bm), uintptr(unsafe.Pointer(&bm)))
		procDeleteObject.Call(info.HbmColor)
		procDeleteObject.Call(info.HbmMask)
		procDestroyIcon.Call(icon)
		if n == 0 {
			t.Fatalf("shape %d has no color bitmap", shape)
		}
		if bm.Width != 32 || bm.Height != 32 {
			t.Errorf("shape %d is %dx%d, want 32x32", shape, bm.Width, bm.Height)
		}
	}
}

// The icon's soft edges came out black: the bitmap held plain white at a lower
// alpha, which the PNG encoder read as premultiplied and wrote out dark, so
// Windows drew every anti-aliased pixel with a black fringe.
func TestTaskbarIconEdgesStayWhite(t *testing.T) {
	for _, shape := range []int{taskbarShapePrev, taskbarShapePlay, taskbarShapePause, taskbarShapeNext, taskbarShapeHeart, taskbarShapeHeartOn} {
		data, err := taskbarIconPNG(shape, 16)
		if err != nil {
			t.Fatalf("shape %d: %v", shape, err)
		}
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("shape %d: %v", shape, err)
		}
		soft := 0
		for y := range 16 {
			for x := range 16 {
				c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
				if c.A == 0 || c.A == 0xff {
					continue
				}
				soft++
				if c.R != 0xff || c.G != 0xff || c.B != 0xff {
					t.Fatalf("shape %d edge pixel %d,%d is rgb(%d,%d,%d) at alpha %d, want white", shape, x, y, c.R, c.G, c.B, c.A)
				}
			}
		}
		if soft == 0 {
			t.Errorf("shape %d has no soft edge pixels", shape)
		}
	}
}
