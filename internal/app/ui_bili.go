package app

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/bilibili"
	"netease-music-mygo/internal/diag"
	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/player"
)

// biliBase is where a track of Bilibili's is in the IDs of the other
// sources: local files take the range above it.
const biliBase = 1 << 48

// The API of Bilibili, which tests replace.
var (
	biliSearch    = bilibili.Search
	biliResolve   = bilibili.Resolve
	biliPlayURL   = bilibili.PlayURL
	biliSubtitles = bilibili.Subtitles
)

// biliLinkRe matches what names one video: a link, a BV or an av number.
var biliLinkRe = regexp.MustCompile(`^(https?://|b23\.tv/|(bv[0-9a-z]{10}|av[0-9]+)$)`)

// biliLink reports whether q names a video rather than words to search for.
func biliLink(q string) bool {
	return biliLinkRe.MatchString(strings.ToLower(strings.TrimSpace(q)))
}

// biliTrack maps a video to the track the pages list and the player plays:
// its title is the song, its uploader the artist.
func (a *app) biliTrack(v bilibili.Video) netease.Track {
	id := -int64(biliBase) - v.Aid
	if a.biliVideos == nil {
		a.biliVideos, a.biliRemotes = map[int64]bilibili.Video{}, map[int64]player.Remote{}
	}
	a.biliVideos[id] = v
	a.biliRemotes[id] = player.Remote{Resolve: func() (player.Remote, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		audio, err := biliPlayURL(ctx, v)
		if err != nil {
			return player.Remote{}, err
		}
		// Lyrics are the video's subtitles, when it has any.
		lrc, _ := biliSubtitles(ctx, v)
		return player.Remote{URL: audio.URL, Headers: audio.Headers, Lyrics: lrc}, nil
	}}
	return netease.Track{
		ID:       id,
		Name:     v.Title,
		Artists:  []netease.Artist{{ID: v.UpMid, Name: v.Up}},
		Album:    netease.Album{PicURL: v.Cover},
		Duration: v.Duration.Milliseconds(),
		// Nothing of NetEase's is asked of it, and it stays able to play.
		Privilege: &netease.Privilege{Pl: 999000},
	}
}

// biliSliceBase is where the IDs of a video's stretches begin: below the
// videos', and below the files of this machine, so none of them meet.
const biliSliceBase = 1 << 49

// biliSliceID is the ID of the stretch of a video that starts at startMs,
// the same on every load.
func biliSliceID(bvid string, startMs int64) int64 {
	h := fnv.New64a()
	io.WriteString(h, bvid)
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], uint64(startMs))
	h.Write(b[:])
	return -int64(biliSliceBase) - int64(h.Sum64()&(biliSliceBase/2-1))
}

// biliSliceTrack maps a stretch of a video, named by its tracklist, to the
// track the pages list: it plays from start to end, of the video's own
// sound.
func (a *app) biliSliceTrack(v bilibili.Video, name string, start, end int64) netease.Track {
	id := biliSliceID(v.Bvid, start)
	if a.biliVideos == nil {
		a.biliVideos, a.biliRemotes = map[int64]bilibili.Video{}, map[int64]player.Remote{}
	}
	a.biliVideos[id] = v
	a.biliRemotes[id] = player.Remote{
		Start: time.Duration(start) * time.Millisecond,
		End:   time.Duration(end) * time.Millisecond,
		Resolve: func() (player.Remote, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			audio, err := biliPlayURL(ctx, v)
			if err != nil {
				return player.Remote{}, err
			}
			return player.Remote{URL: audio.URL, Headers: audio.Headers}, nil
		},
	}
	if strings.TrimSpace(name) == "" {
		name = v.Title
	}
	return netease.Track{
		ID:        id,
		Name:      name,
		Artists:   []netease.Artist{{ID: v.UpMid, Name: v.Up}},
		Album:     netease.Album{Name: v.Title, PicURL: v.Cover},
		Duration:  end - start,
		Privilege: &netease.Privilege{Pl: 999000},
	}
}

// loadBili searches videos and uploaders with one query: the words, the
// video a link names, or the uploader it names.
func (a *app) loadBili(query string) {
	query = strings.TrimSpace(query)
	if query == "" {
		return
	}
	// Another search is a new look: the preview the last video made goes,
	// so the same video makes a fresh one.
	a.biliPreview = nil
	a.biliQuery, a.biliShown, a.biliLoading, a.biliErr = query, query, true, ""
	a.biliUsers = nil
	go func() {
		defer diag.Recover("bili")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var videos []bilibili.Video
		var users []bilibili.User
		var verr, uerr error
		if biliLink(query) {
			var v bilibili.Video
			if v, verr = biliResolve(ctx, query); verr == nil {
				videos = []bilibili.Video{v}
			}
		} else {
			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer wg.Done()
				videos, verr = biliSearch(ctx, query, 1)
			}()
			go func() {
				defer wg.Done()
				users, uerr = biliSearchUsers(ctx, query, 1)
			}()
			wg.Wait()
		}
		a.post(func() {
			a.biliLoading = false
			if verr != nil && uerr != nil {
				diag.Errorf("bilibili search %q: %v", query, verr)
				a.biliErr = biliMessage(verr)
				a.redraw()
				return
			}
			a.biliTracks = a.biliTracks[:0]
			a.biliHits = a.biliHits[:0]
			for _, v := range videos {
				tr := a.biliTrack(v)
				a.biliTracks = append(a.biliTracks, tr)
				a.biliHits = append(a.biliHits, v)
				a.probeBiliSource(tr.ID, v)
			}
			a.biliUsers = users
			a.biliErr = ""
			a.redraw()
		})
	}()
}

// biliMessage says what went wrong with a request of Bilibili's.
func biliMessage(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "请求哔哩哔哩超时，请稍后重试"
	case errors.Is(err, bilibili.ErrRiskControl):
		return "哔哩哔哩暂时限制了请求，请稍后重试"
	}
	if err != nil {
		return err.Error()
	}
	return ""
}

// A biliSource is what NetEase has for the song a video names: it is
// looked for once the video is shown, so a page can say whether it is
// there instead of guessing.
type biliSource struct {
	done  bool
	track *netease.Track
}

// probeBiliSource looks the song a video's title names up on NetEase, once
// per video.
func (a *app) probeBiliSource(id int64, v bilibili.Video) {
	if a.biliSources == nil {
		a.biliSources = map[int64]biliSource{}
	}
	if _, ok := a.biliSources[id]; ok {
		return
	}
	a.biliSources[id] = biliSource{}
	go func() {
		defer diag.Recover("bili-source")
		t := biliFind(biliProbeQuery(v.Title))
		name := ""
		if t != nil {
			name = t.Name + " · " + t.ArtistNames()
		}
		diag.Logf("bili source %d (%s): %q", id, v.Bvid, name)
		a.post(func() {
			a.biliSources[id] = biliSource{done: true, track: t}
		})
	}()
}

// biliProbeQuery is the words a video's title gives to look its song up
// by: the title in book quotes with the name beside it, or the title.
func biliProbeQuery(title string) string {
	title = strings.TrimSpace(title)
	if m := biliQuoteRe.FindStringSubmatch(title); m != nil {
		name := biliClean(m[1])
		before := strings.TrimSpace(title[:strings.Index(title, m[0])])
		if i := strings.LastIndexAny(before, "｜|"); i >= 0 {
			before = before[i+len("｜"):]
		}
		before = strings.TrimSpace(strings.TrimRight(before, "-–—:： "))
		if before != "" {
			return name + " - " + before
		}
		return name
	}
	return biliClean(title)
}

// biliSourceOf is what the site has for a video's song, when it has been
// looked for.
func (a *app) biliSourceOf(id int64) biliSource {
	return a.biliSources[id]
}

// biliHeader is the page's head: its name, what may be done with the feed,
// and the field searching videos and uploaders alike.
func (a *app) biliHeader(c *ui.Context) {
	p := a.pal
	ui.Row(c).AlignItems(ui.Center).Gap(10).Margin(0, 0, 16, 0).Children(func() {
		ui.Text(c, "哔哩哔哩").FontSize(headTitle).FontWeight(800).LetterSpacing(-0.6).LineHeight(1.2).Grow(1)
		if a.biliShown != "" {
			if pillButton(c.Key("bili-back"), nil, "返回关注", p.hover.Over(p.bg), p.text).Clicked() {
				a.backToBiliFeed()
			}
		} else if len(a.follows) > 0 {
			if iconButton(c.Key("bili-refresh"), iconReload, 18, p.muted, "刷新投稿").Size(36, 36).Clicked() {
				a.refreshBiliFeed()
			}
		}
		if a.biliAcc == nil {
			if pillButton(c.Key("bili-signin"), nil, "登录 B 站", p.hover.Over(p.bg), p.text).Clicked() {
				a.openBiliLogin()
			}
		} else if pillButton(c.Key("bili-import"), iconFolder, "导入收藏", p.hover.Over(p.bg), p.text).Clicked() {
			a.openBiliImport()
		}
	})
	ui.Row(c).Height(topBarHeight).Margin(0, 0, 20, 0).Children(func() {
		field := ui.Row(c).Height(topBarHeight).PaddingX(12).Gap(8).Radius(radiusField).
			Grow(1).MaxWidth(560).AlignItems(ui.Center)
		if c.Focused(a.biliBox) {
			field.Background(p.bg).Shadow(0, 0, 0, 3, p.accent.Alpha(0.25)).Border(1, p.accent.Alpha(0.6))
		} else {
			field.Background(p.hover).Border(1, p.hairline)
		}
		field.Children(func() {
			ui.Icon(c, iconSearch).Size(17, 17).TextColor(p.muted)
			in := ui.TextInputBase(c, &a.biliQuery).Bind(&a.biliBox).Grow(1).MinWidth(0).FontSize(14).
				Placeholder("搜索视频、UP 主或 BV 号").Label("搜索哔哩哔哩")
			if in.Submitted() {
				a.loadBili(a.biliQuery)
			}
			if a.biliQuery != "" {
				iconButton(c.Key("bili-clear"), iconX, 16, p.muted, "清除搜索").KeepFocus().OnClick(func() {
					a.biliQuery = ""
					a.biliBox.Focus(c)
					a.redraw()
				})
			}
		})
	})
	switch {
	case a.biliErr != "":
		ui.Text(c, a.biliErr).FontSize(14).TextColor(p.muted).Margin(0, 0, 8, 0)
	case a.biliFeedErr != "" && len(a.follows) > 0 && a.biliShown == "":
		ui.Text(c, a.biliFeedErr).FontSize(14).TextColor(p.muted).Margin(0, 0, 8, 0)
	}
}

// biliPage is the uploaders followed turned into playlists, like the page
// of recommendations, and the search over videos and uploaders.
func (a *app) biliPage(c *ui.Context) {
	if len(a.biliNew) > 0 {
		a.markBiliSeen()
	}
	if len(a.follows) > 0 && time.Since(a.biliPulled) > 5*time.Minute {
		a.refreshBiliFeed()
	}
	switch {
	case a.biliShown != "":
		// What was searched for comes first, over the feed: a video a link
		// names may be one no uploader followed posted.
		a.biliResults(c)
	case len(a.follows) == 0:
		a.biliEmpty(c)
	default:
		a.biliHome(c)
	}
}

// biliHome is the feed of the uploaders followed: their latest videos, each
// a card that opens its playlist.
func (a *app) biliHome(c *ui.Context) {
	videos := a.biliListed()
	a.scrollPage(c, func() {
		a.biliHeader(c)
		a.biliFollowStrip(c)
		if len(videos) == 0 {
			if a.biliLoadingFeed {
				a.section(c, "最新投稿")
				cols := a.columns(c, 220)
				a.videoCardsSkeleton(c, a.pageShimmer(c), cols*2, cols)
				return
			}
			a.emptyState(c, "还没有新的投稿", "点右上角的刷新再看一次", "刷新", a.refreshBiliFeed)
			return
		}
		a.section(c, a.biliSectionTitle())
		ui.Grid(c).Columns(a.columns(c, 220)).GapX(20).GapY(24).Children(func() {
			for _, v := range videos {
				a.biliSongCard(c, v)
			}
		})
	})
}

// biliFollowStrip is the uploaders followed, each a round face: a click
// lists that uploader's videos alone, and another click lists them all
// again. The face carries the way to stop following it under the pointer.
func (a *app) biliFollowStrip(c *ui.Context) {
	if len(a.follows) == 0 {
		return
	}
	p := a.pal
	a.section(c, fmt.Sprintf("关注 · %d", len(a.follows))).Margin(0, 0, 12, 0)
	ui.Row(c).Wrap().Gap(16).Margin(0, 0, 24, 0).Children(func() {
		a.biliFollowCell(c, "bili-all", "", "全部", "全部", a.biliFilter == 0, true,
			p.accent.Alpha(0.12), p.hover, func(c *ui.Context, face ui.Element) {
				ui.Icon(c, iconVideo).Size(20, 20).TextColor(p.text)
			}, func() { a.biliFilter = 0 })
		for _, f := range a.follows {
			f := f
			a.biliFollowCell(c, fmt.Sprint(f.Mid), f.Name, f.Name, "查看 "+f.Name+" 的歌单", a.biliFilter == f.Mid, false,
				ui.Transparent, ui.Transparent, func(c *ui.Context, face ui.Element) {
					inner := ui.Box(c).Size(50, 50).Radius(radiusPill).Shadow(0, 4, 12, -4, p.cardShade)
					inner.Children(func() {
						a.coverFill(c, f.Face, 180, radiusPill)
					})
					if face.Hovered() {
						off := ui.ButtonBase(c.Key("bili-off")).Absolute().Top(0).Right(0).Size(20, 20).
							Radius(radiusPill).Background(p.bg).Border(1, p.hairline).Center().
							Label("取消关注 " + f.Name)
						off.Children(func() {
							ui.Icon(c, iconX).Size(10, 10).TextColor(p.muted)
						})
						mid := f.Mid
						off.OnClick(func() {
							a.unfollowBili(mid)
						})
					}
				}, func() {
					if a.biliFilter == f.Mid {
						a.biliFilter = 0
					} else {
						a.biliFilter = f.Mid
					}
				})
		}
	})
}

// biliFollowCell is a cell of the strip: the round button picking what the
// page lists, its ring lit while it is picked, and the name under it. inside
// draws what the button holds; pick runs when it is clicked. A name is bold
// when the cell is picked, or when bold says it always is. The button is
// filled idle, or lit while picked.
func (a *app) biliFollowCell(c *ui.Context, key, cellLabel, name, label string, on, bold bool, lit, idle ui.Color, inside func(c *ui.Context, face ui.Element), pick func()) {
	p := a.pal
	cell := ui.Column(c.Key(key)).Width(60).Gap(6).AlignItems(ui.Center)
	if cellLabel != "" {
		cell.Label(cellLabel)
	}
	cell.Children(func() {
		face := ui.ButtonBase(c.Key("bili-face")).Size(56, 56).Radius(radiusPill).Padding(2).Center().
			Background(idle).Label(label)
		switch {
		case on:
			face.Border(2, p.accent).Background(lit)
		case face.Hovered():
			face.Border(2, p.hairline)
		}
		face.Children(func() { inside(c, face) })
		if face.Clicked() {
			pick()
		}
		ink := p.muted
		if on {
			ink = p.text
		}
		text := ui.Text(c, name).FontSize(12).TextColor(ink).SingleLine().
			TextAlign(ui.Center).AlignSelf(ui.Stretch)
		if on || bold {
			text.FontWeight(600)
		}
	})
}

// biliListed is the feed as the page lists it: all the videos of the
// uploaders followed, or those of the uploader picked.
func (a *app) biliListed() []bilibili.Video {
	if a.biliFilter == 0 {
		return a.biliFeed
	}
	var out []bilibili.Video
	for _, v := range a.biliFeed {
		if v.UpMid == a.biliFilter {
			out = append(out, v)
		}
	}
	return out
}

// biliSectionTitle says what the cards below are: everybody's, or one
// uploader's.
func (a *app) biliSectionTitle() string {
	if a.biliFilter == 0 {
		return "最新投稿"
	}
	for _, f := range a.follows {
		if f.Mid == a.biliFilter {
			return f.Name + " 的投稿"
		}
	}
	return "最新投稿"
}

// biliSongCard is a video's card, as a playlist's is, its cover wide: it
// opens the playlist the video makes, or makes it first, which the card
// says it is doing.
func (a *app) biliSongCard(c *ui.Context, v bilibili.Video) {
	sub := v.Up + " · " + time.Unix(v.PubDate, 0).Format("01-02")
	if a.biliBuilding[v.Bvid] {
		sub = "正在生成歌单…"
	}
	if a.videoCard(c.Key("bili-"+v.Bvid), v.Title, v.Cover, sub, func() { a.openBiliVideo(v, true) }) {
		a.openBiliVideo(v, false)
	}
}

// biliResults lists what the search found: uploaders first, then videos.
func (a *app) biliResults(c *ui.Context) {
	if a.biliLoading {
		a.trackSkeleton(c, a.pageShimmer(c), true, a.biliHeader)
		return
	}
	a.scrollPage(c, func() {
		a.biliHeader(c)
		if len(a.biliUsers) > 0 {
			a.section(c, "UP 主")
			for _, u := range a.biliUsers {
				a.biliUserRow(c, u)
			}
		}
		if len(a.biliHits) > 0 {
			a.section(c, "视频")
			ui.Grid(c).Columns(a.columns(c, 220)).GapX(20).GapY(24).Children(func() {
				for _, v := range a.biliHits {
					a.biliSongCard(c, v)
				}
			})
		}
		if len(a.biliUsers) == 0 && len(a.biliHits) == 0 {
			msg := "没有找到相关视频或 UP 主"
			if a.biliErr != "" {
				msg = a.biliErr
			}
			a.emptyState(c, msg, "", "返回关注", a.backToBiliFeed)
		}
	})
}

// biliUserRow is an uploader an uploader search found, with what it takes
// to follow them here or stop.
func (a *app) biliUserRow(c *ui.Context, u bilibili.User) {
	p := a.pal
	followed := a.followed(u.Mid)
	ui.Row(c.Key(u.Mid)).Height(64).Padding(8, 10).Gap(12).Radius(radiusRow).AlignItems(ui.Center).Children(func() {
		a.cover(c, u.Face, 120, 48, radiusPill)
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(3).Children(func() {
			ui.Text(c, u.Name).FontSize(16).FontWeight(600).SingleLine()
			line := fmt.Sprintf("%d 粉丝", u.Fans)
			if u.Sign != "" {
				line += " · " + u.Sign
			}
			ui.Text(c, line).FontSize(14).TextColor(p.faint).SingleLine()
		})
		if followed {
			if pillButton(c.Key("bili-off"), nil, "取消关注", p.hover.Over(p.bg), p.text).Clicked() {
				a.unfollowBili(u.Mid)
			}
		} else if pillButton(c.Key("bili-on"), nil, "关注", p.accent, p.onAccent).Clicked() {
			a.followBili(u)
		}
	})
}

// backToBiliFeed puts the feed back where a search's results were.
func (a *app) backToBiliFeed() {
	a.biliShown, a.biliQuery, a.biliTracks, a.biliHits, a.biliUsers = "", "", nil, nil, nil
	a.biliErr = ""
	a.redraw()
}

// biliEmpty is the page before an uploader is followed.
func (a *app) biliEmpty(c *ui.Context) {
	a.scrollPage(c, func() {
		a.biliHeader(c)
		a.emptyState(c, "还没有关注 UP 主", "搜一个 UP 主，关注后他的投稿会变成歌单", "", nil)
	})
}
