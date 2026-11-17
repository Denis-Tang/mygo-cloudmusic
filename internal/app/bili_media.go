package app

// The sound of a video, kept here for the stretches a tracklist names: cut
// into a file of its own with FFmpeg, where it is installed, or kept whole
// with each stretch playing from its own place in it.

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/bilibili"
	"netease-music-mygo/internal/diag"
	"netease-music-mygo/internal/local"
	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/player"
)

// The API, which tests replace.
var (
	lookFFmpeg = func() (string, bool) {
		path, err := exec.LookPath("ffmpeg")
		return path, err == nil
	}
	runFFmpeg = func(ctx context.Context, ffmpeg string, args []string) error {
		cmd := exec.CommandContext(ctx, ffmpeg, args...)
		hideWindow(cmd)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	}
)

// biliSlicesDir is where a video's stretches are kept as files of their
// own, beside the listener's own music in the app's data.
var biliSlicesDir = func() (string, error) {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "切片"), nil
}

// biliAudioDir is where a video's whole sound is kept, once.
var biliAudioDir = func() (string, error) {
	dir, err := mygo.App.Path(mygo.PathCache)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "bili"), nil
}

// A biliSliceAsk asks whether to cut the stretches of a playlist into
// files, which needs FFmpeg.
type biliSliceAsk struct {
	open     bool
	playlist int64
	count    int
	err      string
}

// biliStretches are the items of a playlist that play a stretch of a video,
// and the count of them.
func biliStretches(pl *biliPlaylist) []int {
	var out []int
	for i := range pl.Items {
		it := &pl.Items[i]
		if it.Video != nil && it.End > it.Start {
			out = append(out, i)
		}
	}
	return out
}

// prepareBiliPlaylist keeps the sound of the videos a playlist stretches
// out: cut into files where FFmpeg is installed, or kept whole otherwise.
func (a *app) prepareBiliPlaylist(id int64) {
	pl := a.biliPlaylist(id)
	if pl == nil {
		return
	}
	stretches := biliStretches(pl)
	if len(stretches) == 0 {
		return
	}
	if ffmpeg, ok := lookFFmpeg(); ok {
		a.runBiliSlices(id, ffmpeg)
		return
	}
	a.biliSliceAsk = biliSliceAsk{open: true, playlist: id, count: len(stretches)}
	a.redraw()
}

// runBiliSlices keeps what the playlist's stretches need, on a goroutine:
// with ffmpeg, each is cut into a file of its own; without it, the video's
// sound is kept whole and each stretch plays from it.
func (a *app) runBiliSlices(id int64, ffmpeg string) {
	pl := a.biliPlaylist(id)
	if pl == nil {
		return
	}
	stretches := biliStretches(pl)
	items := make([]biliItem, len(stretches))
	for k, i := range stretches {
		items[k] = pl.Items[i]
	}
	a.toast = "正在准备音轨…"
	go func() {
		defer diag.Recover("bili-slices")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		dir, err := biliAudioDir()
		if err != nil {
			a.post(func() { a.toast = "无法保存音轨：没有可用的缓存目录" })
			return
		}
		files := map[string]string{}
		cut, kept := 0, 0
		for _, it := range items {
			v := *it.Video
			src, ok := files[v.Bvid]
			if !ok {
				src, err = biliAudioFile(ctx, dir, v)
				if err != nil {
					diag.Errorf("bili audio %s: %v", v.Bvid, err)
					a.post(func() { a.toast = "音轨下载失败：" + biliMessage(err) })
					return
				}
				files[v.Bvid] = src
			}
			if ffmpeg != "" {
				path, err := cutBiliSlice(ctx, ffmpeg, src, it)
				if err != nil {
					diag.Errorf("bili slice %q: %v", it.Track.Name, err)
					continue
				}
				a.post(func() { a.keepBiliSliceFile(id, it.Track.Name, path) })
				cut++
				continue
			}
			a.post(func() {
				a.biliLocals[it.Track.ID] = player.Local{
					File:  src,
					Start: time.Duration(it.Start) * time.Millisecond,
					End:   time.Duration(it.End) * time.Millisecond,
				}
			})
			kept++
		}
		a.post(func() {
			switch {
			case ffmpeg != "" && cut > 0:
				a.toast = fmt.Sprintf("已切成 %d 个音频文件，可在本地音乐里找到", cut)
				a.loadLibrary()
			case ffmpeg != "":
				a.toast = "音轨切片失败，见日志"
			case kept > 0:
				a.toast = fmt.Sprintf("已缓存 %d 段音轨，可直接播放", kept)
			}
			a.redraw()
		})
	}()
}

// keepBiliSliceFile puts the file a stretch was cut into among the
// playlist's rows, and among the music of this machine.
func (a *app) keepBiliSliceFile(id int64, name, path string) {
	pl := a.biliPlaylist(id)
	if pl == nil {
		return
	}
	cacheDir, _ := mygo.App.Path(mygo.PathCache)
	t, ok := local.Read(path, cacheDir)
	if !ok {
		return
	}
	for i := range pl.Items {
		it := &pl.Items[i]
		if it.Video == nil || it.Track.Name != name || it.End <= it.Start {
			continue
		}
		it.Track = localTrack(t)
		it.Start, it.End = 0, 0
		a.biliLocals[it.Track.ID] = player.Local{File: t.Path, Cover: t.Cover, LRC: t.LRC}
		break
	}
	a.saveBiliPlaylists()
}

// sliceFileName is the name a stretch takes as a file: the song's, without
// what a file name cannot hold.
func sliceFileName(name string) string {
	name = strings.TrimSpace(name)
	replacer := strings.NewReplacer("/", "／", "\\", "＼", ":", "：", "*", "＊", "?", "？", "\"", "＂", "<", "＜", ">", "＞", "|", "｜")
	name = replacer.Replace(name)
	if name == "" {
		name = "音轨"
	}
	if rs := []rune(name); len(rs) > 80 {
		name = string(rs[:80])
	}
	return name
}

// cutBiliSlice cuts one stretch out of a video's sound, into a file of its
// own in the slices folder.
func cutBiliSlice(ctx context.Context, ffmpeg, src string, it biliItem) (string, error) {
	dir, err := biliSlicesDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	name := sliceFileName(it.Track.Name)
	path := filepath.Join(dir, name+".m4a")
	// A file cut before, of the same stretch, is that one: collecting a
	// playlist again cuts nothing twice.
	cacheDir, _ := mygo.App.Path(mygo.PathCache)
	for n := 2; ; n++ {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			break
		}
		if t, ok := local.Read(path, cacheDir); ok {
			if d := (it.End - it.Start) - t.Duration.Milliseconds(); d > -1500 && d < 1500 {
				return path, nil
			}
		}
		path = filepath.Join(dir, name+" ("+strconv.Itoa(n)+").m4a")
	}
	start := seconds(it.Start)
	end := seconds(it.End)
	artist := ""
	if len(it.Track.Artists) > 0 {
		artist = it.Track.Artists[0].Name
	}
	args := []string{
		"-y", "-ss", start, "-to", end, "-i", src,
		"-vn", "-c", "copy",
		"-metadata", "title=" + it.Track.Name,
	}
	if artist != "" {
		args = append(args, "-metadata", "artist="+artist)
	}
	if it.Track.Album.Name != "" {
		args = append(args, "-metadata", "album="+it.Track.Album.Name)
	}
	args = append(args, path)
	if err := runFFmpeg(ctx, ffmpeg, args); err != nil {
		return "", err
	}
	// The cover beside the file, as the library reads one.
	if it.Track.Album.PicURL != "" {
		if b, err := netease.Fetch(netease.Sized(it.Track.Album.PicURL, 1200)); err == nil {
			os.WriteFile(strings.TrimSuffix(path, ".m4a")+".jpg", b, 0o600)
		}
	}
	return path, nil
}

// seconds writes a moment as ffmpeg reads it.
func seconds(ms int64) string {
	return strconv.FormatFloat(float64(ms)/1000, 'f', 3, 64)
}

// bindBiliSlice makes a stretch play from the sound kept on this machine,
// where it was kept whole.
func (a *app) bindBiliSlice(v bilibili.Video, id int64, start, end int64) {
	dir, err := biliAudioDir()
	if err != nil {
		return
	}
	path := filepath.Join(dir, v.Bvid+".m4a")
	if info, err := os.Stat(path); err != nil || info.Size() == 0 {
		return
	}
	if a.biliLocals == nil {
		a.biliLocals = map[int64]player.Local{}
	}
	a.biliLocals[id] = player.Local{
		File:  path,
		Start: time.Duration(start) * time.Millisecond,
		End:   time.Duration(end) * time.Millisecond,
	}
}

// biliAudioFile keeps a video's sound whole under dir, once, and returns
// its file.
func biliAudioFile(ctx context.Context, dir string, v bilibili.Video) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, v.Bvid+".m4a")
	if info, err := os.Stat(path); err == nil && info.Size() > 0 {
		return path, nil
	}
	audio, err := biliPlayURL(ctx, v)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, audio.URL, nil)
	if err != nil {
		return "", err
	}
	for k, val := range audio.Headers {
		req.Header.Set(k, val)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	// The request asks from the start of the sound, so the CDN may answer
	// with a 206 of all of it.
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusPartialContent {
		return "", fmt.Errorf("音频下载失败（HTTP %d）", res.StatusCode)
	}
	tmp := path + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, res.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return "", err
	}
	return path, os.Rename(tmp, path)
}

// biliSliceDialog asks whether to cut a playlist's stretches into files,
// which needs FFmpeg.
func (a *app) biliSliceDialog(c *ui.Context) {
	p := a.pal
	d := &a.biliSliceAsk
	ui.DialogBase(c.Key("bili-slice"), &d.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.35))
		panel.Width(420).Padding(24).Radius(radiusPanel).Gap(12).
			Background(p.bg).Border(1, p.hairline).Shadow(0, 24, 60, -12, ui.RGBA(0, 0, 0, 0.4)).
			Transition(ui.ElementTransition{Animation: &ui.Snappy, Enter: &ui.Motion{Scale: 0.94}, Exit: &ui.Motion{Scale: 0.96}})
		ui.Text(c, "把音轨切成独立文件").FontSize(17).FontWeight(700)
		ui.Text(c, fmt.Sprintf("这个歌单有 %d 段音轨。装好 FFmpeg 后，它们会切成独立音频文件，并出现在本地音乐里；不装则缓存整段音频，只在哔哩哔哩歌单里播放。", d.count)).
			FontSize(14).TextColor(p.muted).LineHeight(1.5)
		if d.err != "" {
			ui.Text(c, d.err).FontSize(13).TextColor(p.muted)
		}
		ui.Row(c).Gap(10).Justify(ui.End).Children(func() {
			if pillButton(c.Key("later"), nil, "先缓存播放", p.hover.Over(p.bg), p.text).Clicked() {
				d.open = false
				a.runBiliSlices(d.playlist, "")
			}
			if pillButton(c.Key("download"), nil, "打开下载页", p.hover.Over(p.bg), p.text).Clicked() {
				mygo.Shell.OpenExternal("https://ffmpeg.org/download.html")
			}
			if pillButton(c.Key("retry"), nil, "已安装，重试", p.accent, p.onAccent).Clicked() {
				if ffmpeg, ok := lookFFmpeg(); ok {
					d.open = false
					a.runBiliSlices(d.playlist, ffmpeg)
					return
				}
				d.err = "还没有找到 ffmpeg，请确认它已加入 PATH"
			}
		})
	})
}

// hideWindow stops a program the app starts from opening a console window,
// on Windows.
func hideWindow(cmd *exec.Cmd) {
	if runtime.GOOS != "windows" {
		return
	}
	cmd.SysProcAttr = hiddenProcAttr()
}
