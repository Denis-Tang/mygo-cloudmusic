package app

import (
	"slices"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

var collectAlbum = netease.CollectAlbum

// The page of the listener's collected albums, as the sidebar's 我的收藏
// opens it: the covers the listener keeps, the last collected first.

// loadCollected asks for the albums collected, once; it runs on a
// goroutine and answers on the main thread's posts.
func (a *app) loadCollected() {
	if a.collectedLoaded || a.collectedLoading {
		return
	}
	a.collectedLoading, a.collectedErr = true, ""
	profile := a.profile
	go func() {
		albums, err := netease.CollectedAlbums()
		a.post(func() {
			if a.profile != profile {
				return
			}
			a.collectedLoading = false
			if err != nil {
				a.collectedErr = "无法连接网易云音乐，请检查网络"
				return
			}
			a.collected = albums
			a.collectedLoaded = true
		})
	}()
}

// collectedPage shows the collected albums as the albums' pages show
// theirs: their covers in a grid, the name and the year under each.
func (a *app) collectedPage(c *ui.Context) {
	if a.collectedErr == "" {
		a.loadCollected()
	}
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

func (a *app) albumCollected(id int64) bool {
	return slices.ContainsFunc(a.collected, func(al netease.AlbumProfile) bool { return al.ID == id })
}

func (a *app) setAlbumCollected(al *netease.AlbumProfile, collect bool) {
	a.collected = slices.DeleteFunc(a.collected, func(p netease.AlbumProfile) bool { return p.ID == al.ID })
	if collect {
		a.collected = slices.Insert(a.collected, 0, *al)
	}
}

// toggleAlbumCollection updates the shared collection, undoing a refused change.
func (a *app) toggleAlbumCollection(al *netease.AlbumProfile) {
	if a.profile == nil {
		a.openLogin()
		return
	}
	profile, collect := a.profile, !a.albumCollected(al.ID)
	a.setAlbumCollected(al, collect)
	go func() {
		err := collectAlbum(al.ID, collect)
		a.post(func() {
			if a.profile != profile {
				return
			}
			if err != nil {
				a.setAlbumCollected(al, !collect)
				a.toast = "操作失败，请稍后再试"
			} else if collect {
				a.toast = "已收藏专辑"
			} else {
				a.toast = "已取消收藏"
			}
		})
	}()
}
