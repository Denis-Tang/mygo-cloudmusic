//go:build !windows

package app

import "netease-music-mygo/internal/netease"

// taskbarControls is the Windows taskbar button's thumbnail toolbar. Other
// platforms have none: their taskbars do not take one.
type taskbarControls struct{}

func (t *taskbarControls) init(hwnd uintptr, a *app) {}

func (t *taskbarControls) refresh() {}

func (t *taskbarControls) sync(tr *netease.Track, playing, liked bool) {}

func (t *taskbarControls) close() {}
