package app

import (
	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// The page of the listener's collected albums, as the sidebar's 我的收藏
// opens it: the covers the listener keeps, the last collected first.

// loadCollected asks for the albums collected, once; it runs on a
// goroutine and answers on the main thread's posts.
func (a *app) loadCollected() {
	if a.collectedLoaded {
		return
	}
	a.collectedLoaded = true
	go func() {
		albums, err := netease.CollectedAlbums()
		a.post(func() {
			if err != nil {
				a.collectedLoaded = false
				a.collectedErr = "无法连接网易云音乐，请检查网络"
				a.redraw()
				return
			}
			a.collected = albums
			a.collectedErr = ""
			a.redraw()
		})
	}()
}

// collectedPage shows the collected albums as the albums' pages show
// theirs: their covers in a grid, the name and the year under each.
func (a *app) collectedPage(c *ui.Context) {
	a.loadCollected()
	a.scrollPage(c, func() {
		a.heading(c, "我的收藏", "收藏的专辑")
		switch {
		case a.collectedErr != "":
			a.centered(c, a.collectedErr, a.loadCollected)
		case a.collectedLoaded && len(a.collected) == 0:
			a.empty(c, "还没有收藏的专辑")
		case len(a.collected) > 0:
			a.albumGrid(c, a.collected)
		default:
			s := a.pageShimmer(c)
			cards := a.columns(c, 168)
			a.playlistCardsSkeleton(c.Key("collected"), s, cards*2, 168, false)
		}
	})
}
