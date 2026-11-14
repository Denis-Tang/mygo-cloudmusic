package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo"

	"netease-music-mygo/internal/bilibili"
	"netease-music-mygo/internal/diag"
	"netease-music-mygo/internal/netease"
)

// A biliFollow is an uploader the listener follows on this machine alone:
// the site is never told, and no cookie or account is needed for it.
type biliFollow struct {
	Mid   int64    `json:"mid"`
	Name  string   `json:"name"`
	Face  string   `json:"face"`
	Sign  string   `json:"sign"`
	Added int64    `json:"added"`
	Seen  []string `json:"seen,omitempty"`
}

// biliSearchMode is what the search field looks for: videos, or uploaders.
type biliSearchMode int

const (
	biliVideos biliSearchMode = iota
	biliUploaders
)

// biliPollEvery is how often the uploaders followed are looked up again.
const biliPollEvery = 15 * time.Minute

// biliSeenMost is how many of an uploader's videos are remembered as seen.
const biliSeenMost = 200

// The API, which tests replace.
var biliSearchUsers = bilibili.SearchUsers

// biliFollowsFile is where the follows are kept.
var biliFollowsFile = func() (string, error) {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "bili_follows.json"), nil
}

// loadBiliFollows reads the uploaders followed, newest first.
func loadBiliFollows() []biliFollow {
	path, err := biliFollowsFile()
	if err != nil {
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var list []biliFollow
	if json.Unmarshal(b, &list) != nil {
		return nil
	}
	return list
}

// saveBiliFollows keeps the list.
func saveBiliFollows(list []biliFollow) error {
	path, err := biliFollowsFile()
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// followed reports whether the uploader is followed.
func (a *app) followed(mid int64) bool {
	for _, f := range a.follows {
		if f.Mid == mid {
			return true
		}
	}
	return false
}

// followBili follows an uploader, keeping the videos the search already
// showed as seen, so following one does not mark everything new.
func (a *app) followBili(u bilibili.User) {
	if a.followed(u.Mid) {
		return
	}
	f := biliFollow{Mid: u.Mid, Name: u.Name, Face: u.Face, Sign: u.Sign, Added: time.Now().Unix()}
	for _, v := range u.Latest {
		f.Seen = append(f.Seen, v.Bvid)
	}
	a.follows = append(a.follows, f)
	if err := saveBiliFollows(a.follows); err != nil {
		diag.Errorf("bili follows: %v", err)
	}
	// The videos the search carried are shown at once, before the next
	// look at the site.
	for _, v := range u.Latest {
		a.addFeedVideo(v)
	}
	diag.Logf("bili follow %d (%s)", u.Mid, u.Name)
	a.refreshBiliFeed()
	a.redraw()
}

// unfollowBili stops following an uploader: only this machine forgets.
func (a *app) unfollowBili(mid int64) {
	kept := a.follows[:0]
	for _, f := range a.follows {
		if f.Mid != mid {
			kept = append(kept, f)
		}
	}
	a.follows = kept
	if err := saveBiliFollows(a.follows); err != nil {
		diag.Errorf("bili follows: %v", err)
	}
	keptFeed := a.biliFeed[:0]
	for _, v := range a.biliFeed {
		if v.UpMid != mid {
			keptFeed = append(keptFeed, v)
		}
	}
	a.biliFeed = keptFeed
	a.rebuildFeedTracks()
	diag.Logf("bili unfollow %d", mid)
	a.redraw()
}

// addFeedVideo puts a video among the followed ones, newest first, unless
// it is there already.
func (a *app) addFeedVideo(v bilibili.Video) {
	for _, have := range a.biliFeed {
		if have.Bvid == v.Bvid {
			return
		}
	}
	a.biliFeed = append(a.biliFeed, v)
	sort.SliceStable(a.biliFeed, func(i, j int) bool { return a.biliFeed[i].PubDate > a.biliFeed[j].PubDate })
}

// rebuildFeedTracks maps the feed to the tracks the pages list, in the
// same order.
func (a *app) rebuildFeedTracks() {
	a.biliFeedTr = a.biliFeedTr[:0]
	for _, v := range a.biliFeed {
		tr := a.biliTrack(v)
		if a.biliNew[v.Bvid] {
			// The row's badge says it came in since the last look.
			tr.Reason = "新"
		}
		if d := v.PubDate; d > 0 {
			tr.Album.Name = time.Unix(d, 0).Format("01-02")
		}
		a.biliFeedTr = append(a.biliFeedTr, tr)
	}
}

// refreshBiliFeed looks up each uploader followed, and keeps what they
// posted since the last look. It runs on a goroutine, and posts the
// answer back.
func (a *app) refreshBiliFeed() {
	if a.biliLoadingFeed || len(a.follows) == 0 {
		return
	}
	a.biliLoadingFeed, a.biliFeedErr = true, ""
	follows := append([]biliFollow(nil), a.follows...)
	seen := map[string]bool{}
	for _, f := range follows {
		for _, id := range f.Seen {
			seen[id] = true
		}
	}
	go func() {
		defer diag.Recover("bili-feed")
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		type found struct {
			mid    int64
			name   string
			face   string
			videos []bilibili.Video
		}
		var got []found
		var firstErr error
		for _, f := range follows {
			users, err := biliSearchUsers(ctx, f.Name, 1)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				diag.Errorf("bili feed %s: %v", f.Name, err)
				continue
			}
			for _, u := range users {
				if u.Mid == f.Mid {
					got = append(got, found{f.Mid, u.Name, u.Face, u.Latest})
					break
				}
			}
		}
		a.post(func() {
			a.biliLoadingFeed = false
			a.biliPulled = time.Now()
			if len(got) == 0 && firstErr != nil {
				a.biliFeedErr = biliMessage(firstErr)
				a.redraw()
				return
			}
			for _, g := range got {
				for i := range a.follows {
					if a.follows[i].Mid == g.mid {
						a.follows[i].Name, a.follows[i].Face = g.name, g.face
					}
				}
				for _, v := range g.videos {
					if !seen[v.Bvid] {
						if a.biliNew == nil {
							a.biliNew = map[string]bool{}
						}
						a.biliNew[v.Bvid] = true
					}
					a.addFeedVideo(v)
				}
			}
			a.rebuildFeedTracks()
			a.biliNewNum = a.countNew()
			a.redraw()
		})
	}()
}

// countNew counts the videos not looked at yet, among those followed.
func (a *app) countNew() int {
	n := 0
	for _, v := range a.biliFeed {
		if a.biliNew[v.Bvid] {
			n++
		}
	}
	return n
}

// markBiliSeen marks the feed as looked at: the badge goes, and the rows
// keep their badge until the next look.
func (a *app) markBiliSeen() {
	if a.biliNewNum == 0 && len(a.biliNew) == 0 {
		return
	}
	unseen := a.biliNew
	a.biliNew = map[string]bool{}
	a.biliNewNum = 0
	for i := range a.follows {
		for bvid := range unseen {
			seen := false
			for _, id := range a.follows[i].Seen {
				if id == bvid {
					seen = true
					break
				}
			}
			if !seen {
				a.follows[i].Seen = append(a.follows[i].Seen, bvid)
			}
		}
		if len(a.follows[i].Seen) > biliSeenMost {
			a.follows[i].Seen = a.follows[i].Seen[len(a.follows[i].Seen)-biliSeenMost:]
		}
	}
	if err := saveBiliFollows(a.follows); err != nil {
		diag.Errorf("bili follows: %v", err)
	}
}

// scheduleBiliPoll looks again every biliPollEvery, while uploaders are
// followed.
func (a *app) scheduleBiliPoll() {
	if a.biliPoll != nil {
		a.biliPoll.Stop()
	}
	if len(a.follows) == 0 {
		return
	}
	a.biliPoll = time.AfterFunc(biliPollEvery, func() {
		mygo.RunOnMain(func() {
			a.refreshBiliFeed()
			a.scheduleBiliPoll()
		})
	})
}

// searchBiliUsers searches uploaders, for the page's follow list.
func (a *app) searchBiliUsers(q string) {
	q = strings.TrimSpace(q)
	if q == "" {
		return
	}
	a.biliUserQ, a.biliLoading, a.biliErr = q, true, ""
	go func() {
		defer diag.Recover("bili-users")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		users, err := biliSearchUsers(ctx, q, 1)
		a.post(func() {
			a.biliLoading = false
			if err != nil {
				diag.Errorf("bilibili users %q: %v", q, err)
				a.biliErr = biliMessage(err)
				a.redraw()
				return
			}
			a.biliUsers = users
			a.biliErr = ""
			a.redraw()
		})
	}()
}

// feedTracks returns the feed filtered by the uploader picked.
func (a *app) feedTracks() []netease.Track {
	if a.biliFilter == 0 {
		return a.biliFeedTr
	}
	out := make([]netease.Track, 0, len(a.biliFeedTr))
	for i, v := range a.biliFeed {
		if v.UpMid == a.biliFilter && i < len(a.biliFeedTr) {
			out = append(out, a.biliFeedTr[i])
		}
	}
	return out
}
