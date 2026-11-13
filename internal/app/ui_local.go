package app

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/local"
	"netease-music-mygo/internal/netease"
)

// localScheme is how a cover of a track on this machine is named, for the
// artwork cache to read from the disk rather than the network.
const localScheme = "file://"

// fileURL names the image at path, as a cover's URL.
func fileURL(path string) string {
	return localScheme + strings.ReplaceAll(filepath.ToSlash(path), "%", "%25")
}

// coverPath returns the file an image URL of this machine names.
func coverPath(u string) (string, bool) {
	if !strings.HasPrefix(u, localScheme) {
		return "", false
	}
	return filepath.FromSlash(strings.ReplaceAll(strings.TrimPrefix(u, localScheme), "%25", "%")), true
}

// loadLibrary reads the music folders again, on a goroutine, and shows what
// it found once read.
func (a *app) loadLibrary() {
	folders := append([]string(nil), a.settings.LocalFolders...)
	cacheDir, _ := mygo.App.Path(mygo.PathCache)
	a.localScanning = true
	go func() {
		lib := local.Open(folders, cacheDir)
		a.post(func() {
			a.lib, a.libReady, a.localScanning = lib, true, false
			a.locals = localTracks(lib)
			a.restoreSession()
			a.redraw()
		})
	}()
}

// localTracks returns what the library holds, as the pages list tracks.
func localTracks(lib *local.Library) []netease.Track {
	if lib == nil {
		return nil
	}
	found := lib.Tracks()
	out := make([]netease.Track, len(found))
	for i, t := range found {
		pic := ""
		if t.Cover != "" {
			pic = fileURL(t.Cover)
		}
		out[i] = netease.Track{
			ID:       t.ID,
			Name:     t.Title,
			Artists:  []netease.Artist{{Name: t.Artist}},
			Album:    netease.Album{Name: t.Album, PicURL: pic},
			Duration: t.Duration.Milliseconds(),
			// Nothing of NetEase's is asked of it, and it stays able to play.
			Privilege: &netease.Privilege{Pl: 999000},
		}
	}
	return out
}

// localPage lists the tracks of the listener's folders, which play as any
// others do.
func (a *app) localPage(c *ui.Context) {
	header := func(c *ui.Context) {
		a.heading(c, "本地音乐", strconv.Itoa(len(a.locals))+" 首")
	}
	if a.localScanning && a.lib == nil {
		a.trackSkeleton(c, a.pageShimmer(c), true, header)
		return
	}
	if len(a.locals) == 0 {
		top, bottom := a.insets(c)
		ui.Column(c).Grow(1).Padding(top, 0, bottom, 0).Center().Gap(12).Children(func() {
			ui.Text(c, "还没有本地音乐").FontSize(20).FontWeight(600)
			ui.Text(c, "在设置里的「本地音乐」中添加存放音乐的文件夹").FontSize(15).TextColor(a.pal.muted)
			if ui.Button(c, "打开设置").Clicked() {
				a.openSettings()
			}
		})
		return
	}
	a.trackRows(c, a.locals, true, header)
}

// showLocalFile shows the file of a track on this machine in its folder.
func (a *app) showLocalFile(id int64) {
	if a.lib == nil {
		return
	}
	if t, ok := a.lib.Track(id); ok {
		mygo.Shell.ShowItemInFolder(t.Path)
	}
}

// addLocalFolders asks for folders, and adds them to the library.
func (a *app) addLocalFolders() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:      a.win,
			Title:       "添加音乐文件夹",
			ButtonLabel: "添加",
			Directory:   true,
			Multiple:    true,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		a.post(func() {
			seen := map[string]bool{}
			for _, f := range a.settings.LocalFolders {
				seen[filepath.Clean(f)] = true
			}
			added := false
			for _, p := range paths {
				p = filepath.Clean(p)
				if seen[p] {
					continue
				}
				seen[p] = true
				a.settings.LocalFolders = append(a.settings.LocalFolders, p)
				added = true
			}
			if !added {
				return
			}
			a.changed()
			a.loadLibrary()
		})
	}()
}

// removeLocalFolder takes a folder out of the library.
func (a *app) removeLocalFolder(folder string) {
	var kept []string
	for _, f := range a.settings.LocalFolders {
		if f != folder {
			kept = append(kept, f)
		}
	}
	a.settings.LocalFolders = kept
	a.changed()
	a.loadLibrary()
}
