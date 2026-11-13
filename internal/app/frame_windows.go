//go:build windows

package app

import "syscall"

var (
	frameUser32      = syscall.NewLazyDLL("user32.dll")
	procRedrawWindow = frameUser32.NewProc("RedrawWindow")
	procSetWindowPos = frameUser32.NewProc("SetWindowPos")
)

// repaintFrame draws the window's frame again: Windows keeps the border,
// and the shadow around it, in the color of the appearance it was drawn in.
// Switched from the dark to the light, the window showed the dark edge it
// had until it was moved or resized.
func repaintFrame(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	const (
		rdwInvalidate   = 0x0001
		rdwFrame        = 0x0400
		rdwUpdateNow    = 0x0100
		swpNoSize       = 0x0001
		swpNoMove       = 0x0002
		swpNoZOrder     = 0x0004
		swpNoActivate   = 0x0010
		swpFrameChanged = 0x0020
	)
	procSetWindowPos.Call(hwnd, 0, 0, 0, 0, 0,
		swpNoSize|swpNoMove|swpNoZOrder|swpNoActivate|swpFrameChanged)
	procRedrawWindow.Call(hwnd, 0, 0, rdwInvalidate|rdwFrame|rdwUpdateNow)
}
