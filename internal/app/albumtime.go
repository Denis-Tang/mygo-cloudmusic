package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"

	"netease-music-mygo/internal/netease"
)

// blockCutoff is the instant from which albums are kept from playing: one
// published on 2026-02-27, Beijing time, or after. The listener chose it.
var blockCutoff = time.Date(2026, 2, 27, 0, 0, 0, 0, beijing)

// albumTimesPath is where the album publish times are kept, in the app's
// cache directory.
func albumTimesPath() string {
	dir, err := mygo.App.Path(mygo.PathCache)
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "album_times.json")
}

// loadAlbumTimes reads the album publish times kept: publish times do not
// change, so what is read is never dropped, only added to.
func (a *app) loadAlbumTimes() {
	if a.pubLoaded {
		return
	}
	a.pubLoaded = true
	a.pubTimes = map[int64]int64{}
	b, err := os.ReadFile(albumTimesPath())
	if err != nil {
		return
	}
	json.Unmarshal(b, &a.pubTimes)
}

// keepAlbumTimes writes the album publish times, if any are new. Like
// everything about them, it is for the main thread.
func (a *app) keepAlbumTimes() {
	if !a.pubDirty || a.quitting {
		return
	}
	b, err := json.Marshal(a.pubTimes)
	if err != nil {
		return
	}
	path := albumTimesPath()
	if path == "" {
		return
	}
	if os.WriteFile(path+".part", b, 0o600) == nil {
		if os.Rename(path+".part", path) == nil {
			a.pubDirty = false
		}
	}
}

// albumTime returns the album's publish time, in UTC milliseconds, kept
// from before: found is false while it is unknown. An unknown album here
// goes on being asked about on the main thread's posts, so that once its
// time is known, what plays and what shows both take it.
func (a *app) albumTime(id int64) (int64, bool) {
	a.loadAlbumTimes()
	if t, ok := a.pubTimes[id]; ok {
		return t, true
	}
	if id > 0 && !a.pubAsked[id] {
		a.pubAsked[id] = true
		go func() {
			t, err := netease.AlbumPublishTime(id)
			a.post(func() {
				if err != nil {
					a.pubAsked[id] = false
					return
				}
				a.pubTimes[id] = t
				a.pubDirty = true
				a.keepAlbumTimes()
				a.redraw()
			})
		}()
	}
	return 0, false
}

// blocked reports whether the track is kept from playing: one whose album
// is known published on 2026-02-27 or after. An album whose time is still
// unknown is let, and its time asked for, so the next pass of it is right.
func (a *app) blocked(t netease.Track) bool {
	t0, known := a.albumTime(t.Album.ID)
	return known && t0 >= blockCutoff.UnixMilli()
}

// albumAskCount asks for as many albums' times at once. The asks go to
// and from the single API, one id each, so more at once asks for no more
// than the app's other lists do.
const albumAskAtOnce = 8

// filterKept keeps songs whose albums were published before 2026-02-27,
// and, unknown still, asks for their album's time, which the song is let
// meanwhile. It is for a goroutine of loading; it hands done what is kept.
func (a *app) filterKept(songs []netease.Track, done func([]netease.Track)) {
	go func() {
		// The album times the cache holds already, read on the main thread.
		kept := make(chan map[int64]int64, 1)
		a.post(func() {
			a.loadAlbumTimes()
			m := make(map[int64]int64, len(a.pubTimes))
			for id, t := range a.pubTimes {
				m[id] = t
			}
			kept <- m
		})
		times := <-kept

		// The albums of the songs that are kept neither known: their times
		// are asked for, several at once, once ever, in the cache.
		unknown := []int64{}
		seen := map[int64]bool{}
		for i := range songs {
			id := songs[i].Album.ID
			if _, ok := times[id]; id > 0 && !ok && !seen[id] {
				seen[id] = true
				unknown = append(unknown, id)
			}
		}
		for i := 0; i < len(unknown); i += albumAskAtOnce {
			batch := unknown[i:min(i+albumAskAtOnce, len(unknown))]
			results := make(chan [2]int64, len(batch))
			for _, id := range batch {
				go func(id int64) {
					t, err := netease.AlbumPublishTime(id)
					if err != nil {
						t = 0
					}
					results <- [2]int64{id, t}
				}(id)
			}
			for range batch {
				result := <-results
				id, t := result[0], result[1]
				if t > 0 {
					times[id] = t
					a.post(func() {
						a.loadAlbumTimes()
						a.pubTimes[id] = t
						a.pubDirty = true
						a.keepAlbumTimes()
					})
				}
			}
		}

		// What the asks found now holds all the times known, known by
		// them. Songs of unknown albums stay, for their times asked for.
		out := make([]netease.Track, 0, len(songs))
		cutoff := blockCutoff.UnixMilli()
		for _, s := range songs {
			if t, known := times[s.Album.ID]; known && t >= cutoff {
				if id := s.Album.ID; id > 0 {
					a.post(func() { a.albumTime(id) })
				}
				continue
			}
			out = append(out, s)
		}
		done(out)
	}()
}
