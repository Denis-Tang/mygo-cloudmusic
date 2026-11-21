package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"rsc.io/qr"

	"netease-music-mygo/internal/bilibili"
	"netease-music-mygo/internal/diag"
	"netease-music-mygo/internal/netease"
)

// A biliAccount is the Bilibili account signed in: the cookie the site gave
// when it was scanned, kept on this machine alone, and who it belongs to.
// The account is read, never written to: nothing of the site's is created,
// changed or removed here.
type biliAccount struct {
	Cookie string `json:"cookie"`
	Mid    int64  `json:"mid"`
	Name   string `json:"name"`
	Face   string `json:"face"`
}

// biliAccountFile is where the account is kept.
var biliAccountFile = func() (string, error) {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "bili_account.json"), nil
}

// The API, which tests replace.
var (
	biliQRCode     = bilibili.QRCode
	biliQRPoll     = bilibili.PollLogin
	biliNav        = bilibili.Nav
	biliFavFolders = bilibili.FavFolders
	biliFavItems   = bilibili.FavItems
	biliFollowings = bilibili.Followings
)

// loadBiliAccount reads the account signed in, if any, and signs the site's
// calls in with it.
func (a *app) loadBiliAccount() {
	path, err := biliAccountFile()
	if err != nil {
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var acc biliAccount
	if json.Unmarshal(b, &acc) != nil || acc.Cookie == "" {
		return
	}
	a.biliAcc = &acc
	bilibili.SetSession(acc.Cookie)
}

// saveBiliAccount keeps the account signed in, or, with nil, forgets it.
func (a *app) saveBiliAccount(acc *biliAccount) {
	a.biliAcc = acc
	path, err := biliAccountFile()
	if err != nil {
		diag.Errorf("bili account: %v", err)
		return
	}
	if acc == nil {
		bilibili.SetSession("")
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			diag.Errorf("bili account: %v", err)
		}
		return
	}
	bilibili.SetSession(acc.Cookie)
	b, err := json.MarshalIndent(acc, "", "  ")
	if err != nil {
		diag.Errorf("bili account: %v", err)
		return
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		diag.Errorf("bili account: %v", err)
	}
}

// signOutBili forgets the account, and the cookie with it.
func (a *app) signOutBili() {
	a.saveBiliAccount(nil)
	a.biliImport = biliImport{}
	a.toast = "已退出 B 站账号"
	a.redraw()
}

// A biliLogin is a sign-in of Bilibili's under way.
type biliLogin struct {
	open     bool
	gen      int
	key      string
	code     *qr.Code
	status   bilibili.QRCodeState
	loading  bool
	err      string
	finished bool
}

// openBiliLogin shows a code to scan with the B 站 app on a phone.
func (a *app) openBiliLogin() {
	a.biliLogin = biliLogin{open: true, gen: a.biliLogin.gen + 1, loading: true}
	gen := a.biliLogin.gen
	go func() {
		defer diag.Recover("bili-login")
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		login, err := biliQRCode(ctx)
		var code *qr.Code
		if err == nil {
			code, err = qr.Encode(login.URL, qr.M)
		}
		a.post(func() {
			if gen != a.biliLogin.gen {
				return
			}
			a.biliLogin.loading = false
			if err != nil {
				a.biliLogin.err = "无法获取二维码，请检查网络"
				return
			}
			a.biliLogin.key, a.biliLogin.code = login.Key, code
			a.pollBiliLogin(gen, login.Key)
		})
	}()
}

// pollBiliLogin asks every two seconds whether the code was scanned, until
// the sign-in ends or the dialog closes.
func (a *app) pollBiliLogin(gen int, key string) {
	go func() {
		defer diag.Recover("bili-login-poll")
		for {
			time.Sleep(2 * time.Second)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			state, cookie, err := biliQRPoll(ctx, key)
			cancel()
			if err != nil {
				// The network may come back: the next look asks again.
				continue
			}
			var acc *biliAccount
			if state == bilibili.QRConfirmed && cookie != "" {
				bilibili.SetSession(cookie)
				acc = &biliAccount{Cookie: cookie}
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				if got, nerr := biliNav(ctx); nerr == nil {
					acc.Mid, acc.Name, acc.Face = got.Mid, got.Name, got.Face
				}
				cancel()
			}
			done := make(chan bool, 1)
			a.post(func() {
				l := &a.biliLogin
				if gen != l.gen || !l.open {
					done <- true
					return
				}
				l.status = state
				if state == bilibili.QRConfirmed {
					l.finished = true
					if acc != nil {
						a.saveBiliAccount(acc)
						a.toast = "已登录 B 站"
					}
					a.closeBiliLoginSoon(gen)
				}
				done <- state == bilibili.QRExpired || l.finished
			})
			if <-done {
				return
			}
		}
	}()
}

// closeBiliLoginSoon closes the dialog a moment after it showed the
// welcome.
func (a *app) closeBiliLoginSoon(gen int) {
	go func() {
		time.Sleep(1200 * time.Millisecond)
		a.post(func() {
			if gen == a.biliLogin.gen {
				a.biliLogin.open = false
				a.openBiliImport()
			}
		})
	}()
}

// A biliImport is the account's collections as they are chosen from.
type biliImport struct {
	open    bool
	loading bool
	folders []bilibili.FavFolder
	busy    string // the collection being imported
	err     string
}

// openBiliImport asks for the account's collections. Without an account it
// asks for one first.
func (a *app) openBiliImport() {
	if a.biliAcc == nil {
		a.openBiliLogin()
		return
	}
	a.biliImport = biliImport{open: true, loading: true}
	acc := *a.biliAcc
	go func() {
		defer diag.Recover("bili-import")
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		folders, err := biliFavFolders(ctx, acc.Mid)
		a.post(func() {
			a.biliImport.loading = false
			if err != nil {
				a.biliImport.err = biliMessage(err)
				a.redraw()
				return
			}
			a.biliImport.folders = folders
			a.redraw()
		})
	}()
}

// importBiliFolder makes a playlist here of every video a collection holds,
// replacing the one an earlier import made of it.
func (a *app) importBiliFolder(f bilibili.FavFolder) {
	if a.biliImport.busy != "" {
		return
	}
	a.biliImport.busy = f.Name
	a.redraw()
	go func() {
		defer diag.Recover("bili-import-folder")
		var videos []bilibili.FavVideo
		for page := 1; page <= 50; page++ {
			// Each page is answered on its own, so a large collection
			// is not cut short by one time for all of them.
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			items, more, err := biliFavItems(ctx, f.ID, page)
			cancel()
			if err != nil {
				a.post(func() {
					a.biliImport.busy = ""
					a.biliImport.err = biliMessage(err)
					a.redraw()
				})
				return
			}
			videos = append(videos, items...)
			if !more {
				break
			}
		}
		a.post(func() {
			a.biliImport.busy = ""
			a.toast = a.keepBiliImport(f, videos)
			a.redraw()
		})
	}()
}

// keepBiliImport writes the videos a collection holds into a playlist kept
// here, the name whole whatever it is.
func (a *app) keepBiliImport(f bilibili.FavFolder, videos []bilibili.FavVideo) string {
	items := make([]biliItem, 0, len(videos))
	for _, v := range videos {
		video := bilibili.Video{
			Bvid: v.Bvid, Aid: v.Aid, Title: v.Title, Up: v.Up, UpMid: v.UpMid,
			Cover: v.Cover, Duration: v.Duration, PubDate: v.PubDate,
		}
		items = append(items, biliItem{Track: a.biliTrack(video), Video: &video})
	}
	for i := range a.biliPlaylists {
		if a.biliPlaylists[i].Remote == f.ID {
			a.biliPlaylists[i].Name = f.Name
			a.biliPlaylists[i].Cover = f.Cover
			a.biliPlaylists[i].Items = items
			a.biliPlaylists[i].Collected = true
			a.saveBiliPlaylists()
			if len(items) == 0 {
				return "收藏夹里没有可导入的视频"
			}
			return fmt.Sprintf("已导入「%s」%d 个视频", f.Name, len(items))
		}
	}
	pl := biliPlaylist{
		ID:        biliNewID(),
		Name:      f.Name,
		Cover:     f.Cover,
		Mode:      "tracks",
		Created:   time.Now().Unix(),
		Collected: true,
		Remote:    f.ID,
		Items:     items,
	}
	a.biliPlaylists = append([]biliPlaylist{pl}, a.biliPlaylists...)
	a.saveBiliPlaylists()
	if len(items) == 0 {
		return "收藏夹里没有可导入的视频"
	}
	return fmt.Sprintf("已导入「%s」%d 个视频", f.Name, len(items))
}

// importBiliFollows adds the uploaders the account follows here, where they
// are followed on this machine alone.
func (a *app) importBiliFollows() {
	acc := a.biliAcc
	if acc == nil {
		return
	}
	a.biliImport.busy = "关注"
	a.redraw()
	go func() {
		defer diag.Recover("bili-import-follows")
		var users []bilibili.User
		for page := 1; page <= 40; page++ {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			got, more, err := biliFollowings(ctx, acc.Mid, page)
			cancel()
			if err != nil {
				a.post(func() {
					a.biliImport.busy = ""
					a.biliImport.err = biliMessage(err)
					a.redraw()
				})
				return
			}
			users = append(users, got...)
			if !more {
				break
			}
		}
		a.post(func() {
			a.biliImport.busy = ""
			added := a.keepBiliFollows(users)
			a.toast = fmt.Sprintf("已导入 %d 位 UP 主", added)
			if added > 0 {
				a.refreshBiliFeed()
			}
			a.redraw()
		})
	}()
}

// keepBiliFollows adds the uploaders not followed yet, and returns how
// many it added.
func (a *app) keepBiliFollows(users []bilibili.User) int {
	added := 0
	for _, u := range users {
		if u.Mid == 0 || a.followed(u.Mid) {
			continue
		}
		a.follows = append(a.follows, biliFollow{
			Mid: u.Mid, Name: u.Name, Face: u.Face, Sign: u.Sign, Added: time.Now().Unix(),
		})
		added++
	}
	if added > 0 {
		if err := saveBiliFollows(a.follows); err != nil {
			diag.Errorf("bili follows: %v", err)
		}
	}
	return added
}

// biliLoginDialog shows the code to scan with the B 站 app on a phone.
func (a *app) biliLoginDialog(c *ui.Context) {
	p := a.pal
	l := &a.biliLogin
	ui.DialogBase(c.Key("bili-login"), &l.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.35))
		panel.Width(360).Padding(24).Radius(radiusPanel).Gap(6).AlignItems(ui.Center).
			Background(p.bg).Border(1, p.hairline).Shadow(0, 24, 60, -12, ui.RGBA(0, 0, 0, 0.4)).
			Transition(ui.ElementTransition{Animation: &ui.Snappy, Enter: &ui.Motion{Scale: 0.94}, Exit: &ui.Motion{Scale: 0.96}})
		title, sub := "扫码登录 B 站", "打开哔哩哔哩 App，扫一扫登录"
		switch {
		case l.finished:
			title, sub = "登录成功", "欢迎回来，"+a.biliName()
		case l.err != "":
			sub = l.err
		case l.status == bilibili.QRScanned:
			title, sub = "扫描成功", "请在手机上确认登录"
		case l.status == bilibili.QRExpired:
			sub = "二维码已失效"
		}
		ui.Text(c, title).FontSize(20).FontWeight(700)
		ui.Text(c, sub).FontSize(14).TextColor(p.muted).TextAlign(ui.Center).Margin(0, 0, 14, 0)

		ui.Box(c).Size(212, 212).Radius(radiusCard).Background(ui.RGB(255, 255, 255)).Border(1, p.hairline).Clip().Children(func() {
			if l.code != nil {
				code := l.code
				opacity := float32(1)
				if l.status == bilibili.QRScanned || l.status == bilibili.QRExpired || l.finished || l.err != "" {
					opacity = 0.12
				}
				ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Opacity(opacity).
					Draw(func(pt *ui.Painter, r ui.Rect) { drawQR(pt, r, code, 14) })
			} else if l.err == "" {
				s := shimmer{base: ui.RGBA(0, 0, 0, 0.08), sheen: ui.RGBA(0, 0, 0, 0.03)}.in(c)
				qrSkeleton(c, s, 14).Absolute().Top(0).Left(0).Right(0).Bottom(0)
			}
			if l.status == bilibili.QRExpired || l.err != "" {
				ui.Column(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Center().Children(func() {
					if pillButton(c.Key("refresh"), iconRepeat, "刷新二维码", p.accent, p.onAccent).Clicked() {
						a.openBiliLogin()
					}
				})
			}
		})
	})
}

// biliImportDialog lists the account's collections to import.
func (a *app) biliImportDialog(c *ui.Context) {
	p := a.pal
	d := &a.biliImport
	ui.DialogBase(c.Key("bili-import"), &d.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.35))
		panel.Width(400).Height(560).Radius(radiusPanel).Clip().Background(p.bg).Border(1, p.hairline).Label("导入 B 站收藏").
			Shadow(0, 24, 60, -12, ui.RGBA(0, 0, 0, 0.4)).
			Transition(ui.ElementTransition{Animation: &ui.Snappy, Enter: &ui.Motion{Scale: 0.96}, Exit: &ui.Motion{Scale: 0.98}})
		panel.Children(func() {
			ui.Row(c).Height(56).PaddingX(24).AlignItems(ui.Center).Gap(10).Children(func() {
				if bmp := bitmap(a.art.get(netease.Sized(a.biliFace(), 80), false)); bmp != nil {
					box := ui.Box(c).Size(28, 28).Radius(radiusPill)
					in := artIn(box, bmp)
					box.Children(func() { artImage(c, bmp, radiusPill, in) })
				}
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Children(func() {
					ui.Text(c, "导入 B 站收藏").FontSize(20).FontWeight(700).SingleLine()
					ui.Text(c, a.biliName()).FontSize(12).TextColor(p.muted).SingleLine()
				})
				if ui.Button(c.Key("bili-logout"), "退出账号").Clicked() {
					a.signOutBili()
				}
				if iconButton(c, iconX, 16, p.muted, "关闭导入").Clicked() {
					d.open = false
				}
			})
			ui.Box(c).Grow(1).MinHeight(0).Children(func() {
				ui.Scroll(c).Fill().Padding(24, 24, 24, 24).Gap(10).Children(func() {
					if d.err != "" {
						ui.Text(c, d.err).FontSize(14).TextColor(p.muted)
					}
					if d.loading {
						ui.Text(c, "正在读取收藏夹…").FontSize(14).TextColor(p.muted)
					}
					for _, f := range d.folders {
						f := f
						ui.Row(c.Key(f.ID)).Height(64).Padding(8, 10).Gap(12).AlignItems(ui.Center).
							Radius(radiusCard).Background(p.sidebar).Children(func() {
							a.cover(c, f.Cover, 120, 48, radiusTile)
							ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(3).Children(func() {
								ui.Text(c, f.Name).FontSize(14).FontWeight(600).SingleLine()
								ui.Text(c, fmt.Sprintf("%d 个视频", f.Count)).FontSize(12).TextColor(p.faint).SingleLine()
							})
							label := "导入"
							if d.busy == f.Name {
								label = "导入中…"
							}
							b := pillButton(c.Key("import"), nil, label, p.hover.Over(p.bg), p.text)
							if d.busy != "" {
								b.Disabled(true)
							}
							if b.Clicked() {
								a.importBiliFolder(f)
							}
						})
					}
					if !d.loading && len(d.folders) == 0 && d.err == "" {
						ui.Text(c, "这个账号还没有收藏夹").FontSize(14).TextColor(p.muted)
					}
					ui.Row(c).Margin(0, 8, 0, 0).Children(func() {
						if ui.Button(c.Key("import-follows"), "导入关注的 UP 主").Disabled(d.busy != "").Clicked() {
							a.importBiliFollows()
						}
					})
				})
			})
		})
	})
}

// biliName is the account's name, or that none is signed in.
func (a *app) biliName() string {
	if a.biliAcc == nil || a.biliAcc.Name == "" {
		return "B 站账号"
	}
	return a.biliAcc.Name
}

// biliFace is the account's avatar.
func (a *app) biliFace() string {
	if a.biliAcc == nil {
		return ""
	}
	return a.biliAcc.Face
}
