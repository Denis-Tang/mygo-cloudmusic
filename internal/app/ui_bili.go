package app

import (
	"context"
	"errors"
	"fmt"
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

// loadBili searches videos: the words, or the video a link names.
func (a *app) loadBili(query string) {
	query = strings.TrimSpace(query)
	if query == "" {
		return
	}
	a.biliQuery, a.biliShown, a.biliLoading, a.biliErr = query, query, true, ""
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
				tr := a.biliTrack(v)
				a.biliTracks = append(a.biliTracks, tr)
				a.probeBiliSource(tr.ID, v)
			}
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

// biliPill is a small rounded choice, lit when it is the one chosen.
func (a *app) biliPill(c *ui.Context, key, label string, on bool) (ui.Element, bool) {
	p := a.pal
	b := ui.ButtonBase(c.Key(key)).Height(30).PaddingX(12).Radius(radiusPill).FocusRing(false).Label(label)
	bg := p.hover
	if on {
		bg = p.accent
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
	color := p.text
	if on {
		color = p.onAccent
	}
	b.Children(func() {
		ui.Text(c, label).FontSize(13.5).FontWeight(600).TextColor(color).SingleLine()
	})
	return b, b.Clicked()
}

// biliHeader is the page's head: its name, the search field, the kind of
// what is searched for, and the uploaders followed.
func (a *app) biliHeader(c *ui.Context) {
	p := a.pal
	sub := "关注 UP 主，他们新的投稿会出现在这里"
	if a.biliMode == biliUploaders {
		sub = "搜索 UP 主，关注只记在这台电脑上"
	} else if len(a.follows) > 0 {
		sub = "关注的 UP 主的新投稿；点开就是视频的音轨"
	}
	a.heading(c, "哔哩哔哩", sub)

	row := ui.Row(c).Height(40).Gap(8).AlignItems(ui.Center).Margin(0, 0, 10, 0)
	row.Children(func() {
		for _, m := range []struct {
			mode  biliSearchMode
			label string
		}{{biliVideos, "视频"}, {biliUploaders, "UP 主"}} {
			if _, clicked := a.biliPill(c, "bili-mode-"+m.label, m.label, a.biliMode == m.mode); clicked {
				a.biliMode = m.mode
				a.biliErr = ""
			}
		}
		field := ui.Row(c).Height(36).PaddingX(12).Gap(8).Radius(radiusField).Background(p.hover).Border(1, p.hairline).Grow(1).AlignItems(ui.Center)
		field.Children(func() {
			ui.Icon(c, iconSearch).Size(15, 15).TextColor(p.muted)
			in := ui.TextInput(c, &a.biliQuery).Grow(1).MinWidth(0).FontSize(14)
			if a.biliMode == biliUploaders {
				in.Placeholder("搜索 UP 主").Label("搜索哔哩哔哩 UP 主")
			} else {
				in.Placeholder("搜索视频，或粘贴链接、BV 号").Label("搜索哔哩哔哩")
			}
			if in.Submitted() {
				if a.biliMode == biliUploaders {
					a.searchBiliUsers(a.biliQuery)
				} else {
					a.loadBili(a.biliQuery)
				}
			}
		})
		if a.biliShown != "" {
			if pillButton(c.Key("bili-back"), nil, "返回关注", p.hover.Over(p.bg), p.text).Clicked() {
				a.backToBiliFeed()
			}
		} else if len(a.follows) > 0 {
			if iconButton(c.Key("bili-refresh"), iconReload, 16, p.muted, "刷新投稿").Clicked() {
				a.refreshBiliFeed()
			}
		}
	})

	if len(a.follows) > 0 && a.biliMode == biliVideos && a.biliShown == "" {
		ui.Row(c).Gap(6).Margin(0, 0, 12, 0).Children(func() {
			type chip struct {
				mid   int64
				label string
			}
			chips := []chip{{0, "全部"}}
			for _, f := range a.follows {
				chips = append(chips, chip{f.Mid, f.Name})
			}
			for _, ch := range chips {
				if _, clicked := a.biliPill(c, fmt.Sprintf("bili-chip-%d", ch.mid), ch.label, a.biliFilter == ch.mid); clicked {
					a.biliFilter = ch.mid
				}
			}
		})
	}

	switch {
	case a.biliLoading || a.biliLoadingFeed:
		ui.Text(c, "正在加载…").FontSize(13).TextColor(p.muted).Margin(0, 0, 8, 0)
	case a.biliErr != "":
		ui.Text(c, a.biliErr).FontSize(13).TextColor(p.muted).Margin(0, 0, 8, 0)
	case a.biliFeedErr != "" && len(a.follows) > 0:
		ui.Text(c, a.biliFeedErr).FontSize(13).TextColor(p.muted).Margin(0, 0, 8, 0)
	}
}

// biliPage lists what the uploaders followed posted, and searches videos
// or uploaders when asked. Without a follow there is nothing to show.
func (a *app) biliPage(c *ui.Context) {
	if len(a.biliNew) > 0 {
		a.markBiliSeen()
	}
	if len(a.follows) > 0 && time.Since(a.biliPulled) > 5*time.Minute {
		a.refreshBiliFeed()
	}
	switch {
	case a.biliMode == biliUploaders:
		a.biliUserResults(c)
	case a.biliShown != "":
		// What was searched for comes first, over the feed: a video a link
		// names may be one no uploader followed posted.
		a.biliResults(c)
	case len(a.follows) == 0:
		a.biliEmpty(c)
	case len(a.feedTracks()) == 0:
		if a.biliLoadingFeed || a.biliLoading {
			a.trackSkeleton(c, a.pageShimmer(c), true, a.biliHeader)
			return
		}
		a.biliEmpty(c)
	default:
		a.trackRows(c, a.feedTracks(), true, a.biliHeader)
	}
}

// biliResults lists what the search found, or says it found nothing.
func (a *app) biliResults(c *ui.Context) {
	if a.biliLoading {
		a.trackSkeleton(c, a.pageShimmer(c), true, a.biliHeader)
		return
	}
	if len(a.biliTracks) > 0 {
		a.trackRows(c, a.biliTracks, true, a.biliHeader)
		return
	}
	top, bottom := a.insets(c)
	ui.Scroll(c.Key("bili-found")).TrackScroll(&a.pageScroll).Grow(1).Padding(top+pageGap, pagePad, bottom+48, pagePad).Children(func() {
		a.startMark(c)
		a.biliHeader(c)
		ui.Column(c).Center().Padding(60, 0, 40, 0).Gap(10).Children(func() {
			msg := "没有找到这个视频"
			if a.biliErr != "" {
				msg = a.biliErr
			}
			ui.Text(c, msg).FontSize(16).TextColor(a.pal.muted)
			if ui.Button(c, "返回关注").Clicked() {
				a.backToBiliFeed()
			}
		})
	})
}

// backToBiliFeed puts the feed back where a search's results were.
func (a *app) backToBiliFeed() {
	a.biliShown, a.biliQuery, a.biliTracks = "", "", nil
	a.biliErr = ""
	a.redraw()
}

// biliEmpty is the page before an uploader is followed, or when the ones
// followed posted nothing the site shows.
func (a *app) biliEmpty(c *ui.Context) {
	top, bottom := a.insets(c)
	ui.Scroll(c.Key("bili-empty")).TrackScroll(&a.pageScroll).Grow(1).Padding(top+pageGap, pagePad, bottom+48, pagePad).Children(func() {
		a.startMark(c)
		a.biliHeader(c)
		ui.Column(c).Center().Padding(60, 0, 40, 0).Gap(10).Children(func() {
			if len(a.follows) == 0 {
				ui.Text(c, "还没有关注 UP 主").FontSize(18).FontWeight(600)
				ui.Text(c, "切到「UP 主」搜索并关注，他们新的投稿就会出现在这里").FontSize(14).TextColor(a.pal.muted)
				if ui.Button(c, "去搜索 UP 主").Clicked() {
					a.biliMode = biliUploaders
				}
				return
			}
			ui.Text(c, "还没有新的投稿").FontSize(18).FontWeight(600)
			ui.Text(c, "点右上角的刷新再看一次").FontSize(14).TextColor(a.pal.muted)
			if ui.Button(c, "刷新").Clicked() {
				a.refreshBiliFeed()
			}
		})
	})
}

// biliUserResults lists the uploaders an uploader search found, each with
// what it takes to follow them here.
func (a *app) biliUserResults(c *ui.Context) {
	p := a.pal
	top, bottom := a.insets(c)
	ui.Scroll(c.Key("bili-users")).TrackScroll(&a.pageScroll).Grow(1).Padding(top+pageGap, pagePad, bottom+48, pagePad).Children(func() {
		a.startMark(c)
		a.biliHeader(c)
		if a.biliLoading {
			ui.Text(c, "正在搜索…").FontSize(14).TextColor(p.muted).Padding(20, 0)
			return
		}
		if len(a.biliUsers) == 0 {
			ui.Text(c, "没有找到相关 UP 主").FontSize(14).TextColor(p.muted).Padding(20, 0)
			return
		}
		for _, u := range a.biliUsers {
			u := u
			followed := a.followed(u.Mid)
			ui.Row(c.Key(u.Mid)).Height(76).Padding(10, 12).Gap(12).Radius(radiusRow).AlignItems(ui.Center).Children(func() {
				a.cover(c, u.Face, 120, 48, 24)
				ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(3).Children(func() {
					ui.Text(c, u.Name).FontSize(16).FontWeight(600).SingleLine()
					line := fmt.Sprintf("%d 粉丝", u.Fans)
					if u.Sign != "" {
						line += " · " + u.Sign
					}
					ui.Text(c, line).FontSize(13).TextColor(p.muted).SingleLine()
				})
				if followed {
					if ui.Button(c, "已关注").Clicked() {
						a.unfollowBili(u.Mid)
					}
				} else if ui.Button(c, "关注").Clicked() {
					a.followBili(u)
				}
			})
		}
	})
}
