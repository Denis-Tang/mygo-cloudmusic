package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"
)

// A temporary download failure used to stay cached forever while its cover was visible.
func TestArtCacheRecoversFromFailure(t *testing.T) {
	b, err := os.ReadFile("testdata/animated.webp")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Write(b)
	}))
	defer s.Close()
	posts := make(chan func(), 4)
	c := newArtCache(func(fn func()) { posts <- fn })
	now := time.Unix(1e9, 0)
	c.now, c.swept = func() time.Time { return now }, now
	c.get(s.URL, false)
	wait(t, posts, "failed cover")()
	now = now.Add(artRetry / 2)
	if a := c.get(s.URL, false); a == nil || !a.failed {
		t.Fatal("did not retain the failure during the retry delay")
	}
	now = now.Add(artRetry / 2)
	c.get(s.URL, false)
	wait(t, posts, "recovered cover")()
	if a := c.get(s.URL, false); a == nil || a.failed || a.bmp == nil || calls != 2 {
		t.Fatalf("cover did not recover after two requests: %+v, requests %d", a, calls)
	}
}

// Requests from pages already left used to queue ahead of the current page's covers.
func TestArtCacheDoesNotQueueOldPages(t *testing.T) {
	b, err := os.ReadFile("testdata/animated.webp")
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{}, 1)
	defer close(release)
	asked := make(chan string, 64)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked <- r.URL.Path
		if r.URL.Path == "/first" {
			<-release
		}
		w.Write(b)
	}))
	defer s.Close()
	posts := make(chan func(), 64)
	c := newArtCache(func(fn func()) { posts <- fn })
	c.sem = make(chan struct{}, 1)
	c.get(s.URL+"/first", false)
	wait(t, asked, "first cover")
	for i := range 40 {
		c.get(s.URL+"/old/"+strconv.Itoa(i), false)
	}
	queued := len(c.busy)
	release <- struct{}{}
	if queued != 1 {
		t.Fatalf("queued %d requests while only one could start", queued)
	}
	wait(t, posts, "first cover loaded")()
	c.get(s.URL+"/current", false)
	if path := wait(t, asked, "current page cover"); path != "/current" {
		t.Fatalf("loaded %s before the current page", path)
	}
	wait(t, posts, "current page loaded")()
	if bitmap(c.get(s.URL+"/current", false)) == nil {
		t.Fatal("current page cover did not load")
	}
}

// An animated WebP cover, as some playlists have, shows its first frame,
// in the bitmap too, which decodes its data again once it drops its pixels.
func TestAnimatedWebPCover(t *testing.T) {
	b, err := os.ReadFile("testdata/animated.webp")
	if err != nil {
		t.Fatal(err)
	}
	bmp, img, err := decodeArt(b)
	if err != nil {
		t.Fatal(err)
	}
	if r := img.Bounds(); r.Dx() != 24 || r.Dy() != 24 {
		t.Errorf("first frame is %v, want 24×24", r)
	}
	if w, h := bmp.Size(); w != 24 || h != 24 {
		t.Errorf("bitmap is %d×%d, want 24×24", w, h)
	}
}

// Covers not asked for in a while go, so that a long session does not
// keep every cover it showed; the ones still showing stay.
func TestArtCacheDropsUnusedCovers(t *testing.T) {
	now := time.Unix(1e9, 0)
	c := newArtCache(func(func()) {})
	c.now, c.swept = func() time.Time { return now }, now
	c.art["shown"] = &artwork{used: now}
	c.art["gone"] = &artwork{used: now}
	for range 7 {
		now = now.Add(time.Minute)
		c.get("shown", false)
	}
	if _, ok := c.art["gone"]; ok {
		t.Error("kept a cover not asked for in 7 minutes")
	}
	if _, ok := c.art["shown"]; !ok {
		t.Error("dropped a cover asked for every minute")
	}
}

// The covers showing when the window went idle stay for its next frame.
// They went: nothing asks for covers while the window draws nothing, so
// after 5 minutes away the first frame, as the window was focused again,
// dropped every cover on the page, which flashed as they loaded again.
func TestArtCacheKeepsCoversWhileIdle(t *testing.T) {
	now := time.Unix(1e9, 0)
	c := newArtCache(func(func()) {})
	c.now, c.swept = func() time.Time { return now }, now
	c.art["shown"] = &artwork{used: now}
	c.get("shown", false)
	now = now.Add(10 * time.Minute)
	if c.get("shown", false) == nil {
		t.Error("dropped the covers showing after 10 minutes idle")
	}
}
