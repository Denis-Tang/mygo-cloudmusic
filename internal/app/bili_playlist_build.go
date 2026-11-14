package app

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/bilibili"
	"netease-music-mygo/internal/diag"
)

// A biliDraft is a video turned into songs, waiting for the listener to
// say yes.
type biliDraft struct {
	open    bool
	video   *bilibili.Video
	cands   []biliCand
	loading bool
	ai      bool
	err     string
}

// startBiliPlaylist reads a video's parts, text and the uploader's
// comment, guesses the songs, and looks each up on NetEase.
func (a *app) startBiliPlaylist(v bilibili.Video) {
	a.biliDraft = biliDraft{open: true, loading: true, video: &v}
	go func() {
		defer diag.Recover("bili-draft")
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		full := v
		if got, err := biliResolve(ctx, v.Bvid); err == nil {
			full = got
		}
		comment, _ := bilibili.UpperComments(ctx, full)
		if biliAISong(full.Desc) || biliAISong(comment) {
			// Music the uploader made with a machine is not on the site,
			// so it is not looked for.
			a.post(func() {
				a.biliDraft.loading = false
				a.biliDraft.ai = true
				a.redraw()
			})
			return
		}
		chapters, _ := bilibili.Chapters(ctx, full)
		cands := biliExtract(full, comment, chapters)
		if len(cands) == 0 {
			a.post(func() {
				a.biliDraft.loading = false
				a.biliDraft.err = "没有从视频里找到歌名"
				a.redraw()
			})
			return
		}
		biliMatch(cands)
		a.post(func() {
			a.biliDraft.loading = false
			a.biliDraft.cands = cands
			a.biliDraft.err = ""
			a.redraw()
		})
	}()
}

// biliMatch looks every candidate up on NetEase and keeps the track that
// names the same song.
func biliMatch(cands []biliCand) {
	work := make(chan int)
	n := 4
	if len(cands) < n {
		n = len(cands)
	}
	var wg sync.WaitGroup
	for w := 0; w < n; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range work {
				cands[i].Track = biliFind(cands[i].Raw)
			}
		}()
	}
	for i := range cands {
		work <- i
	}
	close(work)
	wg.Wait()
}

// createBiliDraft makes the playlist the draft lists: matched songs from
// NetEase, the video itself for those nothing matched. Repeats stay.
func (a *app) createBiliDraft() {
	d := &a.biliDraft
	if d.video == nil {
		return
	}
	pl := biliPlaylist{
		ID:      time.Now().UnixNano(),
		Name:    strings.TrimSpace(d.video.Title),
		Origin:  d.video.URL(),
		Mode:    "songs",
		Created: time.Now().Unix(),
	}
	missing := d.ai
	for _, c := range d.cands {
		if c.Track != nil {
			pl.Items = append(pl.Items, biliItem{Track: *c.Track})
		} else {
			missing = true
		}
	}
	if missing {
		// What did not match is not thrown away: the video itself joins
		// the matched songs, as one more item.
		if len(pl.Items) == 0 {
			pl.Mode = "tracks"
		}
		pl.Items = append(pl.Items, biliItem{Track: a.biliTrack(*d.video), Video: d.video})
	}
	a.biliPlaylists = append([]biliPlaylist{pl}, a.biliPlaylists...)
	a.saveBiliPlaylists()
	diag.Logf("bili playlist %q: %d items, mode %s", pl.Name, len(pl.Items), pl.Mode)
	d.open = false
	a.redraw()
	a.open(route{page: pageBiliPlaylist, id: pl.ID})
}

// biliDraftDialog shows what was guessed, and asks whether to keep it.
func (a *app) biliDraftDialog(c *ui.Context) {
	p := a.pal
	d := &a.biliDraft
	ui.DialogBase(c.Key("bili-draft"), &d.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.35))
		panel.Width(520).MaxHeight(560).Padding(24).Radius(radiusPanel).Gap(12).
			Background(p.bg).Border(1, p.hairline).Shadow(0, 24, 60, -12, ui.RGBA(0, 0, 0, 0.4)).
			Transition(ui.ElementTransition{Animation: &ui.Snappy, Enter: &ui.Motion{Scale: 0.94}, Exit: &ui.Motion{Scale: 0.96}})
		ui.Text(c, "从视频生成歌单").FontSize(17).FontWeight(700)
		if d.video != nil {
			ui.Text(c, d.video.Title).FontSize(14).TextColor(p.muted).MaxLines(2)
		}
		switch {
		case d.loading:
			ui.Text(c, "正在读视频里的歌名，并在网易云里查找…").FontSize(14).TextColor(p.muted)
		case d.ai:
			ui.Text(c, "这是 AI 创作的歌曲，网易云上没有；创建后整条视频存成音轨").FontSize(14).TextColor(p.muted)
		case d.err != "":
			ui.Text(c, d.err).FontSize(14).TextColor(p.muted)
		default:
			matched := 0
			for _, cd := range d.cands {
				if cd.Track != nil {
					matched++
				}
			}
			ui.Text(c, "找到 "+strconv.Itoa(len(d.cands))+" 首，匹配到 "+strconv.Itoa(matched)+" 首").FontSize(14).TextColor(p.muted)
			ui.Scroll(c).MaxHeight(300).Children(func() {
				for i, cd := range d.cands {
					if i >= 40 {
						ui.Text(c, "…还有 "+strconv.Itoa(len(d.cands)-40)+" 首").FontSize(13).TextColor(p.faint)
						break
					}
					ui.Row(c).Height(30).Gap(8).AlignItems(ui.Center).Children(func() {
						name := cd.Raw
						if cd.Track != nil {
							name = cd.Track.Name
						}
						ui.Text(c, name).FontSize(14).Grow(1).SingleLine()
						if cd.Track != nil {
							ui.Text(c, "网易云 · "+cd.Track.ArtistNames()).FontSize(12).TextColor(p.accent)
						} else {
							ui.Text(c, "网易云无音源").FontSize(12).TextColor(p.faint)
						}
					})
				}
			})
		}
		ui.Row(c).Gap(10).Justify(ui.End).Children(func() {
			if pillButton(c.Key("cancel"), nil, "取消", p.hover.Over(p.bg), p.text).Clicked() {
				d.open = false
			}
			ok := pillButton(c.Key("ok"), nil, "创建歌单", p.accent, p.onAccent)
			if d.loading || (len(d.cands) == 0 && !d.ai) {
				ok.Disabled(true)
			}
			if ok.Clicked() {
				a.createBiliDraft()
			}
		})
	})
}
