package app

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// searchResults and searchSuggest search NetEase; tests replace them.
var (
	searchResults = netease.SearchPage
	searchSuggest = netease.SearchSuggest
)

// searchLimit is how many results load at a time.
const searchLimit = 60

// searchKinds are the kinds of search, as their tabs show them.
var searchKinds = []struct {
	kind  netease.SearchType
	label string
}{
	{0, "综合"},
	{netease.SearchSongs, "单曲"},
	{netease.SearchPlaylists, "歌单"},
	{netease.SearchAlbums, "专辑"},
	{netease.SearchArtists, "歌手"},
	{netease.SearchUsers, "用户"},
}

// searchData is what a search found, as far as its pages have loaded.
type searchData struct {
	res  netease.SearchResult
	page paged
}

// search searches for q, of the kind last searched for.
func (a *app) search(q string) {
	q = strings.TrimSpace(q)
	if q == "" {
		return
	}
	kind := a.searched.kind
	a.searchFor(route{page: pageSearch, query: q, kind: kind})
}

// searchFor opens the page of a search, remembering its query. Searched
// for again, it loads anew.
func (a *app) searchFor(r route) {
	a.rememberSearch(r.query)
	a.dismissSuggestions()
	if r == a.route {
		a.nowPlaying = false
		a.loadSearch(r, true)
		return
	}
	a.open(r)
}

// loadSearch loads the first page of a search, unless it has, or again.
func (a *app) loadSearch(r route, again bool) {
	if r.kind == 0 {
		for _, kind := range searchKinds[1:5] {
			r.kind = kind.kind
			a.loadSearch(r, again)
		}
		return
	}
	if a.searches[r] != nil && !again {
		return
	}
	d := &searchData{}
	a.searches[r] = d
	a.moreResults(r, d)
}

// moreResults loads the next page of a search. A search done again has
// new data, so the last one's pages land where nothing shows them.
func (a *app) moreResults(r route, d *searchData) {
	if d.page.loading {
		return
	}
	d.page.loading, d.page.err = true, ""
	offset := d.res.Len()
	go func() {
		res, err := searchResults(r.query, r.kind, searchLimit, offset)
		a.post(func() {
			d.page.loading = false
			if err != nil {
				d.page.err = "搜索失败，请稍后再试"
				return
			}
			d.res.Append(res)
			// A server can claim more while sending none.
			d.page.loaded, d.page.more = true, res.Len() > 0 && d.res.Len() < d.res.Total
		})
	}()
}

// searchHistoryLimit is how many searches the history keeps.
const searchHistoryLimit = 20

// rememberSearch puts q first in the history of searches, once.
func (a *app) rememberSearch(q string) {
	history := []string{q}
	for _, s := range a.settings.SearchHistory {
		if s != q && len(history) < searchHistoryLimit {
			history = append(history, s)
		}
	}
	a.settings.SearchHistory = history
	a.settings.save()
}

func (a *app) clearSearchHistory() {
	a.settings.SearchHistory = nil
	a.settings.save()
}

func (a *app) searchPage(c *ui.Context) {
	r := a.route
	if r.kind == 0 {
		a.searchOverview(c)
		return
	}
	d := a.searches[r]
	header := func(c *ui.Context) { a.searchHeader(c) }
	more := func() { a.moreResults(r, d) }
	if r.kind == netease.SearchSongs {
		if !d.page.loaded && d.page.loading {
			a.trackSkeleton(c, a.pageShimmer(c), true, header)
			return
		}
		a.trackRows(c, d.res.Songs, true, header, func(c *ui.Context) {
			a.pageFoot(c, &d.page, more)
			a.searchEmpty(c, d)
		})
		return
	}
	a.scrollPage(c, func() {
		header(c)
		res := &d.res
		switch r.kind {
		case netease.SearchArtists:
			ui.Grid(c).Columns(a.columns(c, 132)).GapX(20).GapY(28).Children(func() {
				for i := range res.Artists {
					a.artistCard(c.Key(res.Artists[i].ID), &res.Artists[i])
				}
			})
		case netease.SearchAlbums:
			a.albumGrid(c, res.Albums)
		case netease.SearchPlaylists:
			ui.Grid(c).Columns(a.columns(c, 168)).GapX(20).GapY(28).Children(func() {
				for i := range res.Playlists {
					a.playlistCard(c.Key(res.Playlists[i].ID), &res.Playlists[i])
				}
			})
		case netease.SearchUsers:
			for i := range res.Users {
				a.userRow(c.Key(res.Users[i].UserID), &res.Users[i])
			}
		}
		if !d.page.loaded && d.page.loading {
			a.playlistCardsSkeleton(c, a.pageShimmer(c), a.columns(c, 156), 156, false)
			return
		}
		a.pageFoot(c, &d.page, more)
		a.searchEmpty(c, d)
	})
}

func (a *app) searchEmpty(c *ui.Context, d *searchData) {
	if d.page.loaded && d.res.Len() == 0 {
		a.empty(c, "没有找到相关结果")
	}
}

// searchHeader shows the original query and the result categories.
func (a *app) searchHeader(c *ui.Context) {
	r := a.route
	at := 0
	for i, k := range searchKinds {
		if k.kind == r.kind {
			at = i
		}
	}
	a.heading(c, r.query, "")
	ui.Row(c).Margin(0, 0, 18, 0).Children(func() {
		labels := make([]string, len(searchKinds))
		for i, k := range searchKinds {
			labels[i] = k.label
		}
		if segmented(c.Key("kind"), &at, labels...) {
			r.kind = searchKinds[at].kind
			a.show(r)
		}
	})
}

// artistCard is an artist's round photo over their name, opening their page.
func (a *app) artistCard(c *ui.Context, ar *netease.ArtistProfile) {
	p := a.pal
	card := ui.ButtonBase(c).Column().Gap(10).AlignItems(ui.Stretch).FocusRing(false).Label(ar.Name)
	hovered := card.Hovered()
	card.Children(func() {
		photo := ar.Avatar
		if photo == "" {
			photo = ar.PicURL
		}
		art := ui.Box(c).AspectRatio(1).Radius(radiusPill)
		if hovered {
			art.Shadow(0, 12, 28, -6, p.cardShade)
		} else {
			art.Shadow(0, 2, 8, -2, p.cardShade)
		}
		art.Children(func() { a.coverFill(c, photo, 400, radiusPill) })
		ui.Text(c, ar.Name).FontSize(16).FontWeight(500).SingleLine().AlignSelf(ui.Center)
	})
	if card.Clicked() {
		a.open(route{page: pageArtist, id: ar.ID})
	}
}

// searchOverview shares loaded result pages with the category tabs.
func (a *app) searchOverview(c *ui.Context) {
	r := a.route
	result := func(kind netease.SearchType) *searchData {
		r.kind = kind
		return a.searches[r]
	}
	a.scrollPage(c, func() {
		a.searchHeader(c)
		albums := result(netease.SearchAlbums)
		if len(albums.res.Albums) > 0 {
			al := &albums.res.Albums[0]
			match := ui.ButtonBase(c.Key("match-album")).Padding(8).Gap(16).Radius(radiusRow).
				AlignItems(ui.Center).Label("专辑：" + al.Name)
			if match.Hovered() {
				match.Background(a.pal.hover)
			}
			match.Children(func() {
				a.cover(c, al.PicURL, 180, 72, radiusTile)
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(6).Children(func() {
					ui.Text(c, al.Name).FontSize(20).FontWeight(700).MaxLines(2)
					ui.Text(c, al.ArtistNames()).FontSize(15).TextColor(a.pal.muted).SingleLine()
				})
			})
			if match.Clicked() {
				a.openPage(route{page: pageAlbum, id: al.ID})
			}
		}
		for _, kind := range searchKinds[1:5] {
			d := result(kind.kind)
			ui.Column(c.Key(kind.kind)).Children(func() {
				if !d.page.loaded && d.page.loading {
					a.section(c, kind.label)
					a.playlistCardsSkeleton(c, a.pageShimmer(c), 2, 156, false)
					return
				}
				if d.page.err != "" {
					a.section(c, kind.label)
					r := a.route
					r.kind = kind.kind
					a.pageFoot(c, &d.page, func() { a.moreResults(r, d) })
					return
				}
				if d.res.Len() == 0 {
					return
				}
				more := ui.ButtonBase(c.Key("more")).AlignSelf(ui.Start).Justify(ui.Start).Margin(28, 0, 12, 0).Gap(6).AlignItems(ui.Center).Label("更多" + kind.label)
				more.Children(func() {
					ui.Text(c, kind.label).FontSize(sectionAt).FontWeight(700)
					ui.Icon(c, iconForward).Size(18, 18).TextColor(a.pal.muted)
				})
				if more.Clicked() {
					r := a.route
					r.kind = kind.kind
					a.show(r)
				}
				switch kind.kind {
				case netease.SearchSongs:
					songs := d.res.Songs[:min(6, len(d.res.Songs))]
					ui.Grid(c).Columns(2).GapX(20).GapY(8).Children(func() {
						for i := range songs {
							a.searchSong(c.Key(songs[i].ID), songs, i)
						}
					})
				case netease.SearchPlaylists:
					ui.Grid(c).Columns(a.columns(c, 168)).GapX(20).GapY(28).Children(func() {
						for i := range min(len(d.res.Playlists), a.columns(c, 168)) {
							a.playlistCard(c.Key(d.res.Playlists[i].ID), &d.res.Playlists[i])
						}
					})
				case netease.SearchAlbums:
					a.albumGrid(c, d.res.Albums[:min(len(d.res.Albums), a.columns(c, 156))])
				case netease.SearchArtists:
					ui.Grid(c).Columns(a.columns(c, 132)).GapX(20).GapY(28).Children(func() {
						for i := range min(len(d.res.Artists), a.columns(c, 132)) {
							a.artistCard(c.Key(d.res.Artists[i].ID), &d.res.Artists[i])
						}
					})
				}
			})
		}
		if songs, lists, artists := result(netease.SearchSongs), result(netease.SearchPlaylists), result(netease.SearchArtists); songs.page.loaded && lists.page.loaded && albums.page.loaded && artists.page.loaded && songs.res.Len()+lists.res.Len()+albums.res.Len()+artists.res.Len() == 0 {
			a.empty(c, "没有找到相关结果")
		}
	})
}

// searchSong plays a related result and keeps its artist links and song menu.
func (a *app) searchSong(c *ui.Context, songs []netease.Track, i int) {
	tr := &songs[i]
	row := ui.ButtonBase(c).Height(80).Padding(8).Gap(12).Radius(radiusRow).
		AlignItems(ui.Center).FocusRing(false).Label(tr.Name)
	if row.Hovered() {
		row.Background(a.pal.hover)
	}
	if !tr.Playable() {
		row.Opacity(0.42)
	}
	row.Children(func() {
		a.cover(c, tr.Album.PicURL, 160, 60, radiusTile)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(6).Children(func() {
			ui.Text(c, tr.Name).FontSize(17).FontWeight(500).SingleLine()
			a.artistLinks(c, tr, a.pal.muted, a.pal.text).FontSize(15).SingleLine()
		})
		a.songMenu(c, row, tr, 0, a.pal.muted)
	})
	if row.Clicked() && tr.Playable() {
		a.pl.PlayQueue(songs, i)
	}
}
