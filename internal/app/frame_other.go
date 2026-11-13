//go:build !windows

package app

// repaintFrame does nothing: only Windows keeps the frame of the appearance
// before a switch.
func repaintFrame(uintptr) {}
