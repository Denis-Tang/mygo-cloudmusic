//go:build windows

package app

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	"netease-music-mygo/internal/netease"
)

// The taskbar button's thumbnail toolbar: what Windows shows over the
// window's preview when the pointer rests on the button. It is the bar's
// controls, as the official client has them, and it works while the window
// is in the taskbar: hidden to the notification area, there is no button to
// carry it.

var (
	taskbarOle32    = syscall.NewLazyDLL("ole32.dll")
	taskbarUser32   = syscall.NewLazyDLL("user32.dll")
	taskbarGdi32    = syscall.NewLazyDLL("gdi32.dll")
	taskbarComctl32 = syscall.NewLazyDLL("comctl32.dll")

	procCoInitializeEx         = taskbarOle32.NewProc("CoInitializeEx")
	procCoCreateInstance       = taskbarOle32.NewProc("CoCreateInstance")
	procRegisterWindowMessageW = taskbarUser32.NewProc("RegisterWindowMessageW")
	procSetWindowSubclass      = taskbarComctl32.NewProc("SetWindowSubclass")
	procRemoveWindowSubclass   = taskbarComctl32.NewProc("RemoveWindowSubclass")
	procDefSubclassProc        = taskbarComctl32.NewProc("DefSubclassProc")
	procDeleteObject           = taskbarGdi32.NewProc("DeleteObject")
	procGetAncestor            = taskbarUser32.NewProc("GetAncestor")
	procGetDpiForWindow        = taskbarUser32.NewProc("GetDpiForWindow")
	procDestroyIcon            = taskbarUser32.NewProc("DestroyIcon")
	procCreateIconFromResource = taskbarUser32.NewProc("CreateIconFromResourceEx")
)

// ITaskbarList3, and the messages of its thumbnail toolbar.
var (
	clsidTaskbarList = taskbarGUID{0x56FDF344, 0xFD6D, 0x11D0, [8]byte{0x95, 0x8A, 0x00, 0x60, 0x97, 0xC9, 0xA0, 0x90}}
	iidTaskbarList3  = taskbarGUID{0xEA1AFB91, 0x9E28, 0x4B86, [8]byte{0x90, 0xE9, 0x9E, 0x9F, 0x8A, 0x5E, 0xEF, 0xAF}}
)

const (
	taskbarListInit      = 3
	taskbarListRelease   = 2
	taskbarAddButtons    = 15
	taskbarUpdateButtons = 16
	thbnClicked          = 0x1800
	thbIcon              = 0x2
	thbTooltip           = 0x4
	thbFlags             = 0x8
	thbfEnabled          = 0x0
	wmCommand            = 0x0111
)

type taskbarGUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

type taskbarIconInfo struct {
	FIcon    int32
	_        uint32
	HbmMask  uintptr
	HbmColor uintptr
}

// taskbarButton is THUMBBUTTON: the mask, the id, the bitmap, the icon,
// the tooltip and the flags, in that order. Windows refuses the buttons
// when the flags come before the tooltip, which is the order the fields
// once seemed to have.
type taskbarButton struct {
	Mask   uint32
	ID     uint32
	Bitmap uint32
	Icon   uintptr
	Tip    [260]uint16
	Flags  uint32
}

type taskbarState struct {
	valid   bool
	id      int64
	playing bool
	liked   bool
}

type taskbarIcons struct {
	made                 bool
	prev, play, pause    uintptr
	next, heart, heartOn uintptr
}

// taskbarControls is the thumbnail toolbar of the window's taskbar button.
type taskbarControls struct {
	app      *app
	hwnd     uintptr
	list     uintptr
	subclass bool
	ready    bool
	added    bool
	failed   bool
	tryAt    time.Time
	state    taskbarState
	iconSize int
	icons    taskbarIcons
	title    string
	thumb    taskbarThumbnail
}

var (
	taskbarApp              *app
	taskbarButtonCreatedMsg uint32
	taskbarSubclassCallback = syscall.NewCallback(taskbarWindowProc)
)

func (t *taskbarControls) init(hwnd uintptr, a *app) {
	if hwnd != 0 {
		root, _, _ := procGetAncestor.Call(hwnd, 2) // GA_ROOT
		if root != 0 {
			hwnd = root
		}
	}
	t.app, t.hwnd = a, hwnd
	taskbarApp = a
	taskbarButtonCreatedMsg = registerTaskbarMessage("TaskbarButtonCreated")
	if hwnd != 0 {
		dpi, _, _ := procGetDpiForWindow.Call(hwnd)
		if dpi == 0 {
			dpi = 96
		}
		t.iconSize = int((16*dpi + 48) / 96)
		if t.iconSize < 16 {
			t.iconSize = 16
		}
		t.thumb.attach(hwnd)
	}
	if hwnd == 0 {
		return
	}
	r, _, _ := procSetWindowSubclass.Call(hwnd, taskbarSubclassCallback, 1, 0)
	t.subclass = r != 0
	// The button may already be there, or Explorer may make it later.
	t.refresh()
	a.win.OnReadyToShow(t.capturePreview)
	a.win.OnMinimize(t.capturePreview)
}

func (t *taskbarControls) capturePreview() {
	bg := t.app.collectPalette().bg
	t.thumb.capture(t.app.win, color.RGBA{bg.R, bg.G, bg.B, 255})
}

func (t *taskbarControls) refresh() {
	if t.app == nil {
		return
	}
	tr := t.app.pl.Current()
	t.setTitle(tr)
	t.thumb.show(tr)
	t.tryAt = time.Time{}
	t.ready = false
	playing := tr != nil && t.app.pl.Playing()
	liked := tr != nil && t.app.isLiked(tr)
	t.sync(tr, playing, liked)
}

// setTitle follows the track with the window's title, which the preview card
// keeps as its header.
func (t *taskbarControls) setTitle(tr *netease.Track) {
	if t.app == nil || t.app.win == nil {
		return
	}
	title := taskbarTitle(tr)
	if title == t.title {
		return
	}
	t.title = title
	t.app.win.SetTitle(title)
}

func (t *taskbarControls) sync(tr *netease.Track, playing, liked bool) {
	if t.hwnd == 0 || t.app == nil {
		return
	}
	t.setTitle(tr)
	t.thumb.show(tr)
	if tr == nil {
		if t.state.valid {
			t.clear()
		}
		return
	}
	if t.ready && t.state.valid && t.state.id == tr.ID && t.state.playing == playing && t.state.liked == liked {
		return
	}
	if t.list == 0 {
		if t.failed {
			return
		}
		t.ensureList()
		if t.list == 0 {
			return
		}
	}
	if !t.ready && time.Since(t.tryAt) < 400*time.Millisecond {
		return
	}
	t.tryAt = time.Now()
	if !t.apply(tr, playing, liked) {
		// The button may not be there yet; the next frame or Explorer's
		// message tries again.
		t.ready = false
		return
	}
	t.ready = true
	t.state = taskbarState{valid: true, id: tr.ID, playing: playing, liked: liked}
}

func (t *taskbarControls) ensureList() {
	if t.list != 0 || t.failed {
		return
	}
	// The window's thread: the toolbar belongs to it. CoInitializeEx may
	// answer that it is initialized already, as MyGo did.
	procCoInitializeEx.Call(0, 2) // COINIT_APARTMENTTHREADED
	var list uintptr
	hr, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidTaskbarList)), 0, 1, // CLSCTX_INPROC_SERVER
		uintptr(unsafe.Pointer(&iidTaskbarList3)), uintptr(unsafe.Pointer(&list)))
	if taskbarFailed(hr) || list == 0 {
		t.failed = true
		return
	}
	if taskbarFailed(comCall(list, taskbarListInit)) {
		t.failed = true
		return
	}
	t.list = list
}

func (t *taskbarControls) apply(tr *netease.Track, playing, liked bool) bool {
	if t.list == 0 {
		return false
	}
	icons := t.makeIcons()
	buttons := make([]taskbarButton, 4)
	set := func(i int, id uint32, icon uintptr, tip string) {
		buttons[i].Mask = thbIcon | thbTooltip | thbFlags
		buttons[i].ID = id
		buttons[i].Icon = icon
		buttons[i].Flags = thbfEnabled
		buttons[i].Tip = taskbarTip(tip)
	}
	play := icons.play
	if playing {
		play = icons.pause
	}
	set(0, 1, icons.prev, "上一首")
	set(1, 2, play, "播放/暂停")
	set(2, 3, icons.next, "下一首")
	heart, tip := icons.heart, "喜欢"
	if liked {
		heart, tip = icons.heartOn, "取消喜欢"
	}
	set(3, 4, heart, tip)
	// The first time adds the buttons; after that their icons change in
	// place. Explorer makes the button again when it starts, which adds
	// them again from TaskbarButtonCreated.
	var hr uintptr
	if t.added {
		hr = comCall(t.list, taskbarUpdateButtons, t.hwnd, 4, uintptr(unsafe.Pointer(&buttons[0])))
	} else {
		hr = comCall(t.list, taskbarAddButtons, t.hwnd, 4, uintptr(unsafe.Pointer(&buttons[0])))
		if !taskbarFailed(hr) {
			t.added = true
		}
	}
	return !taskbarFailed(hr)
}

func (t *taskbarControls) clear() {
	if t.list != 0 && t.added {
		comCall(t.list, taskbarAddButtons, t.hwnd, 0, 0)
		t.added = false
	}
	t.state = taskbarState{}
}

func (t *taskbarControls) makeIcons() *taskbarIcons {
	if t.icons.made {
		return &t.icons
	}
	size := t.iconSize
	if size == 0 {
		size = 16
	}
	t.icons = taskbarIcons{
		made:    true,
		prev:    taskbarIcon(taskbarShapePrev, size),
		play:    taskbarIcon(taskbarShapePlay, size),
		pause:   taskbarIcon(taskbarShapePause, size),
		next:    taskbarIcon(taskbarShapeNext, size),
		heart:   taskbarIcon(taskbarShapeHeart, size),
		heartOn: taskbarIcon(taskbarShapeHeartOn, size),
	}
	return &t.icons
}

func (t *taskbarControls) close() {
	if t.hwnd != 0 && t.subclass {
		procRemoveWindowSubclass.Call(t.hwnd, taskbarSubclassCallback, 1)
		t.subclass = false
	}
	t.clear()
	if t.list != 0 {
		comCall(t.list, taskbarListRelease)
		t.list = 0
	}
	if t.icons.made {
		for _, icon := range []uintptr{t.icons.prev, t.icons.play, t.icons.pause, t.icons.next, t.icons.heart, t.icons.heartOn} {
			if icon != 0 {
				procDestroyIcon.Call(icon)
			}
		}
		t.icons = taskbarIcons{}
	}
	if t.thumb.preview != 0 {
		procDeleteObject.Call(t.thumb.preview)
		t.thumb.preview = 0
	}
	t.thumb.hwnd = 0
	taskbarApp = nil
}

// taskbarWindowProc answers the taskbar button's clicks before MyGo's own
// window procedure sees them, and passes everything else on.
func taskbarWindowProc(hwnd, msg, wParam, lParam, id, ref uintptr) uintptr {
	if a := taskbarApp; a != nil {
		if msg == uintptr(wmCommand) && uint32(wParam)>>16 == thbnClicked {
			id := uint32(wParam) & 0xFFFF
			switch id {
			case 1:
				a.pl.Prev()
			case 2:
				a.pl.TogglePlay()
			case 3:
				a.pl.Next()
			case 4:
				a.likeCurrent()
			}
			a.redraw()
			return 0
		}
		if uint32(msg) == wmSendIconicThumbnail {
			a.taskbar.thumb.reply(lParam)
			a.taskbar.thumb.live()
			a.taskbar.capturePreview()
			return 0
		}
		if uint32(msg) == wmSendIconicLivePreview {
			a.taskbar.thumb.live()
			return 0
		}
		if taskbarButtonCreatedMsg != 0 && uint32(msg) == taskbarButtonCreatedMsg {
			a.taskbar.ready = false
			a.taskbar.added = false
			a.taskbar.tryAt = time.Time{}
			a.taskbar.refresh()
		}
	}
	r, _, _ := procDefSubclassProc.Call(hwnd, msg, wParam, lParam)
	return r
}

func registerTaskbarMessage(name string) uint32 {
	p, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return 0
	}
	r, _, _ := procRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(p)))
	return uint32(r)
}

func comCall(obj uintptr, index int, args ...uintptr) uintptr {
	if obj == 0 {
		return 0
	}
	vtbl := *(*uintptr)(unsafe.Pointer(obj))
	fn := *(*uintptr)(unsafe.Pointer(vtbl + uintptr(index)*unsafe.Sizeof(uintptr(0))))
	all := append([]uintptr{obj}, args...)
	r, _, _ := syscall.SyscallN(fn, all...)
	return r
}

func taskbarFailed(hr uintptr) bool { return int32(uint32(hr)) < 0 }

func taskbarTip(s string) [260]uint16 {
	var out [260]uint16
	for i, r := range utf16.Encode([]rune(s)) {
		if i >= len(out)-1 {
			break
		}
		out[i] = r
	}
	return out
}

// The taskbar's icons: white shapes on transparent, drawn at the size the
// display asks for (16 DIPs at its scale) so Windows does not resample a
// small bitmap, and encoded as PNG icon resources. A DIB icon drew as an
// empty button. A bigger bitmap (20 DIPs at 100%) is shrunk by Explorer with
// a cheap filter, which is what left the glyphs jagged.
// The shapes are drawn on a 16 unit grid: one unit is one pixel of the icon
// at 100% scale, which is the size Explorer draws a thumbnail toolbar's icon.
const taskbarShapeSize = 16

const (
	taskbarShapePlay = iota
	taskbarShapePause
	taskbarShapePrev
	taskbarShapeNext
	taskbarShapeHeart
	taskbarShapeHeartOn
)

func taskbarIconImage(shape, size int) *image.NRGBA {
	if size < 16 {
		size = 16
	}
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	const ss = 16
	scale := float64(taskbarShapeSize) / float64(size)
	for y := range size {
		for x := range size {
			hits := 0
			for sy := range ss {
				for sx := range ss {
					px := (float64(x) + (float64(sx)+0.5)/ss) * scale
					py := (float64(y) + (float64(sy)+0.5)/ss) * scale
					if taskbarShapeAt(shape, px, py) {
						hits++
					}
				}
			}
			if hits == 0 {
				continue
			}
			img.SetNRGBA(x, y, color.NRGBA{255, 255, 255, uint8(255 * hits / (ss * ss))})
		}
	}
	return img
}

// taskbarIconPNG encodes the icon the way Windows reads it: as a PNG, so its
// alpha stays the glyph's coverage rather than a mask.
func taskbarIconPNG(shape, size int) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, taskbarIconImage(shape, size)); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func taskbarIcon(shape, size int) uintptr {
	data, err := taskbarIconPNG(shape, size)
	if err != nil {
		return 0
	}
	icon, _, _ := procCreateIconFromResource.Call(uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)),
		1, 0x00030000, uintptr(max(16, size)), uintptr(max(16, size)), 0)
	return icon
}

// The glyphs follow the official client's: a play triangle with rounded
// points, a bar beside it for skipping, and a heart drawn as an outline.
const (
	taskbarGlyphTop    = 2.5
	taskbarGlyphBottom = 13.5
	taskbarPoint       = 0.7
)

func taskbarShapeAt(shape int, x, y float64) bool {
	switch shape {
	case taskbarShapePlay:
		return taskbarInRoundedTriangle(x, y, 4.5, taskbarGlyphTop, 13.5, 8, 4.5, taskbarGlyphBottom, taskbarPoint)
	case taskbarShapePause:
		return taskbarInRoundedRect(x, y, 3, taskbarGlyphTop, 6, taskbarGlyphBottom, 0.7) ||
			taskbarInRoundedRect(x, y, 10, taskbarGlyphTop, 13, taskbarGlyphBottom, 0.7)
	case taskbarShapePrev:
		return taskbarInRoundedRect(x, y, 1.5, taskbarGlyphTop, 4, taskbarGlyphBottom, 0.6) ||
			taskbarInRoundedTriangle(x, y, 14, taskbarGlyphTop, 6, 8, 14, taskbarGlyphBottom, taskbarPoint)
	case taskbarShapeNext:
		return taskbarInRoundedRect(x, y, 12, taskbarGlyphTop, 14.5, taskbarGlyphBottom, 0.6) ||
			taskbarInRoundedTriangle(x, y, 2, taskbarGlyphTop, 10, 8, 2, taskbarGlyphBottom, taskbarPoint)
	case taskbarShapeHeart:
		return math.Abs(taskbarHeartDistance(x, y)) <= 0.85
	case taskbarShapeHeartOn:
		return taskbarHeartDistance(x, y) <= 0
	}
	return false
}

func taskbarInTriangle(px, py, ax, ay, bx, by, cx, cy float64) bool {
	d1 := (px-bx)*(ay-by) - (ax-bx)*(py-by)
	d2 := (px-cx)*(by-cy) - (bx-cx)*(py-cy)
	d3 := (px-ax)*(cy-ay) - (cx-ax)*(py-ay)
	hasNeg := d1 < 0 || d2 < 0 || d3 < 0
	hasPos := d1 > 0 || d2 > 0 || d3 > 0
	return !(hasNeg && hasPos)
}

// taskbarInRoundedRect is the rectangle grown by r with round corners.
func taskbarInRoundedRect(x, y, x0, y0, x1, y1, r float64) bool {
	cx := min(max(x, x0+r), x1-r)
	cy := min(max(y, y0+r), y1-r)
	dx, dy := x-cx, y-cy
	return dx*dx+dy*dy <= r*r
}

// taskbarInRoundedTriangle is the triangle shrunk by r and grown by r:
// its edges stay where they were, its points turn with a radius.
func taskbarInRoundedTriangle(px, py, ax, ay, bx, by, cx, cy, r float64) bool {
	// The incenter, and how far each vertex moves toward it to inset the
	// edges by r.
	la := math.Hypot(bx-cx, by-cy) // the side opposite a
	lb := math.Hypot(ax-cx, ay-cy) // opposite b
	lc := math.Hypot(ax-bx, ay-by) // opposite c
	per := la + lb + lc
	if per == 0 {
		return false
	}
	ix := (la*ax + lb*bx + lc*cx) / per
	iy := (la*ay + lb*by + lc*cy) / per
	move := func(vx, vy, side1, side2, opposite float64) (float64, float64) {
		if side1 == 0 || side2 == 0 {
			return vx, vy
		}
		// sin of half the angle at the vertex: sqrt((s1+s2-o)(s1+s2+o))/(2 sqrt(s1 s2)).
		sinHalf := math.Sqrt(max(0, (side1+side2-opposite)*(side1+side2+opposite))) / (2 * math.Sqrt(side1*side2))
		if sinHalf <= 0 {
			return vx, vy
		}
		d := r / sinHalf
		dx, dy := ix-vx, iy-vy
		l := math.Hypot(dx, dy)
		if l == 0 || d >= l {
			return ix, iy
		}
		return vx + dx/l*d, vy + dy/l*d
	}
	nax, nay := move(ax, ay, lc, lb, la)
	nbx, nby := move(bx, by, lc, la, lb)
	ncx, ncy := move(cx, cy, lb, la, lc)
	if taskbarInTriangle(px, py, nax, nay, nbx, nby, ncx, ncy) {
		return true
	}
	return taskbarSegment(px, py, nax, nay, nbx, nby) <= r ||
		taskbarSegment(px, py, nbx, nby, ncx, ncy) <= r ||
		taskbarSegment(px, py, ncx, ncy, nax, nay) <= r
}

func taskbarSegment(px, py, ax, ay, bx, by float64) float64 {
	dx, dy := bx-ax, by-ay
	l := dx*dx + dy*dy
	t := 0.0
	if l > 0 {
		t = min(max(((px-ax)*dx+(py-ay)*dy)/l, 0), 1)
	}
	return math.Hypot(px-(ax+t*dx), py-(ay+t*dy))
}

// taskbarHeart is the outline the official client draws: two round lobes, a
// shallow notch between them, and a point at the bottom. It is 13.4 wide and
// 12.3 tall, built from cubic curves so the stroke is the same weight all the
// way round.
var taskbarHeart = taskbarHeartPath()

func taskbarHeartPath() [][2]float64 {
	// start, and then a cubic curve for each of the six sides.
	curves := [][8]float64{
		{8, 14.1, 5.0, 11.6, 1.3, 8.9, 1.3, 5.9},
		{1.3, 5.9, 1.3, 3.4, 3.3, 1.8, 5.3, 1.8},
		{5.3, 1.8, 6.9, 1.8, 7.7, 2.7, 8, 4.3},
		{8, 4.3, 8.3, 2.7, 9.1, 1.8, 10.7, 1.8},
		{10.7, 1.8, 12.7, 1.8, 14.7, 3.4, 14.7, 5.9},
		{14.7, 5.9, 14.7, 8.9, 11.0, 11.6, 8, 14.1},
	}
	const steps = 10
	pts := make([][2]float64, 0, len(curves)*steps+1)
	pts = append(pts, [2]float64{curves[0][0], curves[0][1]})
	for _, c := range curves {
		for i := 1; i <= steps; i++ {
			t := float64(i) / steps
			u := 1 - t
			x := u*u*u*c[0] + 3*u*u*t*c[2] + 3*u*t*t*c[4] + t*t*t*c[6]
			y := u*u*u*c[1] + 3*u*u*t*c[3] + 3*u*t*t*c[5] + t*t*t*c[7]
			pts = append(pts, [2]float64{x, y})
		}
	}
	return pts
}

// taskbarHeartDistance is the distance, in icon pixels, from x, y to the
// heart's outline. Negative is inside.
func taskbarHeartDistance(x, y float64) float64 {
	edge := math.Inf(1)
	for i := 0; i+1 < len(taskbarHeart); i++ {
		a, b := taskbarHeart[i], taskbarHeart[i+1]
		edge = min(edge, taskbarSegment(x, y, a[0], a[1], b[0], b[1]))
	}
	if taskbarInPolygon(x, y, taskbarHeart) {
		return -edge
	}
	return edge
}

// taskbarInPolygon says whether x, y is inside the closed polygon.
func taskbarInPolygon(x, y float64, pts [][2]float64) bool {
	in := false
	for i, j := 0, len(pts)-1; i < len(pts); j, i = i, i+1 {
		yi, yj := pts[i][1], pts[j][1]
		if (yi > y) == (yj > y) {
			continue
		}
		xi, xj := pts[i][0], pts[j][0]
		if x < (xj-xi)*(y-yi)/(yj-yi)+xi {
			in = !in
		}
	}
	return in
}
