package app

import (
	"context"
	"errors"
	"regexp"
	"strings"
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
	biliRanking   = bilibili.Ranking
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

// loadBili finds music videos: the music zone's ranking, or what the
// query searches for, or the video a link names.
func (a *app) loadBili(query string) {
	query = strings.TrimSpace(query)
	if query == "" {
		return
	}
	a.biliQuery, a.biliLoading, a.biliErr = query, true, ""
	go func() {
		defer diag.Recover("bili")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var videos []bilibili.Video
		var err error
		if biliLink(query) {
			var v bilibili.Video
			if v, err = biliResolve(ctx, query); err == nil {
				videos = []bilibili.Video{v}
			}
		} else {
			videos, err = biliSearch(ctx, query, 1)
		}
		a.post(func() {
			a.biliLoading = false
			if err != nil {
				diag.Errorf("bilibili search %q: %v", query, err)
				a.biliErr = biliMessage(err)
				a.redraw()
				return
			}
			a.biliTracks = a.biliTracks[:0]
			for _, v := range videos {
				a.biliTracks = append(a.biliTracks, a.biliTrack(v))
			}
			a.biliErr = ""
			a.redraw()
		})
	}()
}

// loadBiliRanking lists the music zone's ranking until something is
// searched for.
func (a *app) loadBiliRanking() {
	a.biliTried, a.biliLoading = true, true
	go func() {
		defer diag.Recover("bili-ranking")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		videos, err := biliRanking(ctx, 3)
		a.post(func() {
			a.biliLoading = false
			if err != nil {
				diag.Errorf("bilibili ranking: %v", err)
				a.biliErr = biliMessage(err)
				a.redraw()
				return
			}
			for _, v := range videos {
				a.biliTracks = append(a.biliTracks, a.biliTrack(v))
			}
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

// biliHeader is the page's head: its name, and the field to search in.
func (a *app) biliHeader(c *ui.Context) {
	p := a.pal
	a.heading(c, "哔哩哔哩", "视频的音轨，可以作为歌曲播放")
	row := ui.Row(c).Height(40).Gap(8).PaddingX(12).Radius(radiusField).Background(p.hover).Border(1, p.hairline).Margin(0, 0, 14, 0)
	row.Children(func() {
		ui.Icon(c, iconSearch).Size(16, 16).TextColor(p.muted)
		in := ui.TextInput(c, &a.biliQuery).Grow(1).MinWidth(0).
			Placeholder("搜索视频，或粘贴视频链接、BV 号").FontSize(15).Label("搜索哔哩哔哩")
		if in.Submitted() {
			a.loadBili(a.biliQuery)
		}
	})
	switch {
	case a.biliLoading:
		ui.Text(c, "正在加载…").FontSize(14).TextColor(p.muted).Margin(0, 0, 10, 0)
	case a.biliErr != "":
		ui.Text(c, a.biliErr).FontSize(14).TextColor(p.muted).Margin(0, 0, 10, 0)
	}
}

// biliPage searches Bilibili's videos and plays their audio.
func (a *app) biliPage(c *ui.Context) {
	if !a.biliTried && !a.biliLoading {
		a.loadBiliRanking()
	}
	if len(a.biliTracks) == 0 {
		if a.biliLoading {
			a.trackSkeleton(c, a.pageShimmer(c), true, a.biliHeader)
			return
		}
		top, bottom := a.insets(c)
		ui.Scroll(c.Key("bili-empty")).TrackScroll(&a.pageScroll).Grow(1).Padding(top+pageGap, pagePad, bottom+48, pagePad).Children(func() {
			a.startMark(c)
			a.biliHeader(c)
			ui.Column(c).Center().Padding(60, 0, 40, 0).Gap(10).Children(func() {
				ui.Text(c, "还没有内容").FontSize(18).FontWeight(600)
				ui.Text(c, "搜索视频，或粘贴 B 站链接；不登录也能播放音轨").FontSize(14).TextColor(a.pal.muted)
				if ui.Button(c, "重新加载榜单").Clicked() {
					a.biliTracks = nil
					a.biliTried = false
					a.loadBiliRanking()
				}
			})
		})
		return
	}
	a.trackRows(c, a.biliTracks, true, a.biliHeader)
}
