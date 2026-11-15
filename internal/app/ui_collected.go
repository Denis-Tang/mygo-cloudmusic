package app

import (
	"fmt"
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

// collectedPage shows what the listener keeps: the albums collected, the
// playlists of the site collected, and the Bilibili playlists kept here.
func (a *app) collectedPage(c *ui.Context) {
	if a.collectedErr == "" {
		a.loadCollected()
	}
	sub := []string{"收藏的专辑", "收藏的歌单", "收藏的哔哩哔哩歌单"}[a.collectedTab]
	a.scrollPage(c, func() {
		a.heading(c, "我的收藏", sub)
		segmented(c.Key("collected-kind"), &a.collectedTab, "专辑", "歌单", "哔哩哔哩歌单")
		switch a.collectedTab {
		case 1:
			a.collectedPlaylists(c)
		case 2:
			a.collectedBiliPlaylists(c)
		default:
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
		}
	})
}

// collectedPlaylists lists the playlists of the site the listener keeps.
func (a *app) collectedPlaylists(c *ui.Context) {
	if len(a.saved) == 0 {
		a.empty(c, "还没有收藏的歌单")
		return
	}
	ui.Grid(c).Columns(a.columns(c, 168)).GapX(20).GapY(28).Children(func() {
		for i := range a.saved {
			a.playlistCard(c.Key(a.saved[i].ID), &a.saved[i])
		}
	})
}

// collectedBiliPlaylists lists the playlists made from videos and kept
// among what the listener collects: they play here, with the videos' own
// sound, which the site does not hold.
func (a *app) collectedBiliPlaylists(c *ui.Context) {
	var kept []*biliPlaylist
	for i := range a.biliPlaylists {
		if a.biliPlaylists[i].Collected {
			kept = append(kept, &a.biliPlaylists[i])
		}
	}
	if len(kept) == 0 {
		a.empty(c, "还没有收藏的哔哩哔哩歌单")
		return
	}
	ui.Grid(c).Columns(a.columns(c, 168)).GapX(20).GapY(28).Children(func() {
		for _, pl := range kept {
			pl := pl
			cover := pl.Cover
			if cover == "" && len(pl.Items) > 0 {
				cover = pl.Items[0].Track.Album.PicURL
			}
			tracks := biliPlayTracks(pl)
			sub := fmt.Sprintf("%d 首", len(pl.Items))
			if play := func() {
				if a.pl.Shuffle() {
					a.pl.ToggleShuffle()
				}
				a.pl.PlayQueue(tracks, firstPlayable(tracks))
			}; a.coverCard(c.Key(pl.ID), pl.Name, cover, 0, sub, play) {
				a.open(route{page: pageBiliPlaylist, id: pl.ID})
			}
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
