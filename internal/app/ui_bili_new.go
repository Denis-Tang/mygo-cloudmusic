package app

import (
	"context"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/bilibili"
	"netease-music-mygo/internal/diag"
)

// A biliNewPlaylist is a playlist asked for by one video's BV number.
type biliNewPlaylist struct {
	open  bool
	input string
	busy  bool
	err   string
}

// openBiliNew asks for the video a playlist is made from.
func (a *app) openBiliNew() {
	a.biliNewPL = biliNewPlaylist{open: true}
}

// submitBiliNew reads the video the dialog names, and starts what makes a
// playlist of it: its songs are guessed, then asked to be kept.
func (a *app) submitBiliNew() {
	d := &a.biliNewPL
	q := strings.TrimSpace(d.input)
	if q == "" || d.busy {
		return
	}
	if !biliLink(q) {
		d.err = "请输入 BV 号或视频链接"
		return
	}
	d.busy, d.err = true, ""
	go func() {
		defer diag.Recover("bili-new")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		v, err := biliResolve(ctx, q)
		a.post(func() {
			a.biliNewPL.busy = false
			if err != nil {
				a.biliNewPL.err = biliMessage(err)
				a.redraw()
				return
			}
			a.biliNewResolved(v)
		})
	}()
}

// biliNewResolved opens what the video the dialog named makes, or the
// playlist it made before.
func (a *app) biliNewResolved(v bilibili.Video) {
	a.biliNewPL.open = false
	if pl := a.biliPlaylistOf(v.Bvid); pl != nil {
		a.toast = "这个视频的歌单已经在了"
		a.open(route{page: pageBiliPlaylist, id: pl.ID})
		return
	}
	a.startBiliPlaylist(v)
	a.redraw()
}

// biliNewDialog asks for the BV number a playlist is made from.
func (a *app) biliNewDialog(c *ui.Context) {
	p := a.pal
	d := &a.biliNewPL
	ui.DialogBase(c.Key("bili-new"), &d.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.35))
		panel.Width(360).Padding(24).Radius(radiusPanel).Gap(14).
			Background(p.bg).Border(1, p.hairline).Shadow(0, 24, 60, -12, ui.RGBA(0, 0, 0, 0.4)).
			Transition(ui.ElementTransition{Animation: &ui.Snappy, Enter: &ui.Motion{Scale: 0.94}, Exit: &ui.Motion{Scale: 0.96}})
		ui.Text(c, "新建哔哩哔哩歌单").FontSize(20).FontWeight(700)
		in := ui.TextInput(c.Key("bili-new-input"), &d.input).Placeholder("BV 号或视频链接").Label("BV 号或视频链接").FontSize(14).Radius(radiusField).AutoFocus()
		if in.Submitted() {
			a.submitBiliNew()
		}
		if d.err != "" {
			ui.Text(c, d.err).FontSize(14).TextColor(p.muted)
		}
		ui.Row(c).Gap(10).Justify(ui.End).Children(func() {
			if pillButton(c.Key("cancel"), nil, "取消", p.hover.Over(p.bg), p.text).Clicked() {
				d.open = false
			}
			label := "识别"
			if d.busy {
				label = "正在识别…"
			}
			ok := pillButton(c.Key("ok"), nil, label, p.accent, p.onAccent)
			if strings.TrimSpace(d.input) == "" || d.busy {
				ok.Disabled(true)
			}
			if ok.Clicked() {
				a.submitBiliNew()
			}
		})
	})
}
