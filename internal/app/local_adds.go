package app

// Songs of this machine put into the site's playlists: they show and play
// here, first, as the site lists the songs added to a playlist, and the
// site never hears of them, so its own apps do not show them.

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"

	"netease-music-mygo/internal/diag"
	"netease-music-mygo/internal/netease"
)

// localAddsFile is where the songs added to the site's playlists are kept.
var localAddsFile = func() (string, error) {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "local_adds.json"), nil
}

// loadLocalAdds reads the songs this machine added to the site's playlists.
func (a *app) loadLocalAdds() {
	path, err := localAddsFile()
	if err != nil {
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var adds map[int64][]biliItem
	if json.Unmarshal(b, &adds) != nil {
		return
	}
	a.localAdds = adds
	a.restoreLocalAdds()
}

// restoreLocalAdds gives the Bilibili songs added to playlists what plays
// them again, as the playlists kept here are given theirs: a video's track,
// or a stretch of it.
func (a *app) restoreLocalAdds() {
	for pid, items := range a.localAdds {
		for i := range items {
			it := &items[i]
			if it.Video == nil {
				continue
			}
			if it.End > it.Start {
				it.Track = a.biliSliceTrack(*it.Video, it.Track.Name, it.Start, it.End)
				a.bindBiliSlice(*it.Video, it.Track.ID, it.Start, it.End)
				continue
			}
			it.Track = a.biliTrack(*it.Video)
		}
		a.localAdds[pid] = items
	}
}

// saveLocalAdds keeps them.
func (a *app) saveLocalAdds() {
	path, err := localAddsFile()
	if err != nil {
		diag.Errorf("local adds: %v", err)
		return
	}
	b, err := json.MarshalIndent(a.localAdds, "", "  ")
	if err != nil {
		diag.Errorf("local adds: %v", err)
		return
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		diag.Errorf("local adds: %v", err)
	}
}

// mergeLocalAdds puts the songs added here where the site puts the songs
// added to a playlist, first and the newest first, as it shows: the
// playlist reloaded from the site loses them, and takes them again here.
func (a *app) mergeLocalAdds(pl *netease.Playlist) {
	if pl == nil {
		return
	}
	added := a.localAdds[pl.ID]
	if len(added) == 0 {
		return
	}
	mine := make(map[int64]bool, len(added))
	for _, it := range added {
		mine[it.Track.ID] = true
	}
	out := make([]netease.Track, 0, len(pl.Tracks)+len(added))
	for i := len(added) - 1; i >= 0; i-- {
		out = append(out, added[i].Track)
	}
	for _, t := range pl.Tracks {
		if !mine[t.ID] {
			out = append(out, t)
		}
	}
	pl.Tracks = out
}

// addLocalToPlaylist puts a song of this machine into one of the site's
// playlists, on this machine alone.
func (a *app) addLocalToPlaylist(pid int64, tr netease.Track) {
	for _, it := range a.localAdds[pid] {
		if it.Track.ID == tr.ID {
			a.toast = "已经在这个歌单里"
			a.redraw()
			return
		}
	}
	if a.localAdds == nil {
		a.localAdds = map[int64][]biliItem{}
	}
	item := biliItem{Track: tr}
	if v, ok := a.biliVideos[tr.ID]; ok {
		item.Video = &v
		if r, ok := a.biliRemotes[tr.ID]; ok {
			item.Start, item.End = r.Start.Milliseconds(), r.End.Milliseconds()
		}
	}
	a.localAdds[pid] = append(a.localAdds[pid], item)
	a.saveLocalAdds()
	if pl := a.playlists[pid]; pl != nil {
		a.mergeLocalAdds(pl)
	}
	a.plAdding.open = false
	a.toast = "已添加到歌单"
	a.redraw()
}

// isLocalAdd reports whether a song of a site playlist was added here.
func (a *app) isLocalAdd(pid, id int64) bool {
	for _, it := range a.localAdds[pid] {
		if it.Track.ID == id {
			return true
		}
	}
	return false
}

// removeLocalFromPlaylist takes a song added here out of a playlist, on
// this machine alone, and says whether it was one.
func (a *app) removeLocalFromPlaylist(pid, id int64) bool {
	list, ok := a.localAdds[pid]
	if !ok {
		return false
	}
	for i, it := range list {
		if it.Track.ID != id {
			continue
		}
		a.localAdds[pid] = append(list[:i:i], list[i+1:]...)
		if len(a.localAdds[pid]) == 0 {
			delete(a.localAdds, pid)
		}
		a.saveLocalAdds()
		if pl := a.playlists[pid]; pl != nil {
			pl.Tracks = deleteTrack(pl.Tracks, id)
		}
		return true
	}
	return false
}

// deleteTrack returns tracks without the one of id.
func deleteTrack(tracks []netease.Track, id int64) []netease.Track {
	out := make([]netease.Track, 0, len(tracks))
	for _, t := range tracks {
		if t.ID != id {
			out = append(out, t)
		}
	}
	return out
}
