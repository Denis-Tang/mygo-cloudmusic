package netease

import (
	"errors"
	"net/http"
	"net/http/cookiejar"
	"testing"
	"time"
)

// fakeClock runs gate without waiting: sleeping moves its time on.
type fakeClock struct {
	at    time.Time
	slept []time.Duration
}

func newFakeGate() (*gate, *fakeClock) {
	c := &fakeClock{at: time.Unix(1000, 0)}
	g := newGate()
	g.now = func() time.Time { return c.at }
	g.sleep = func(d time.Duration) {
		c.at = c.at.Add(d)
		c.slept = append(c.slept, d)
	}
	g.jitter = func() float64 { return 0.5 }
	g.signedIn = func() bool { return true }
	return g, c
}

// Writes are spaced by writeGap, and more by the jitter, so that a burst of
// changes does not go out at even intervals.
func TestGateSpacesWrites(t *testing.T) {
	g, c := newFakeGate()
	for range 5 {
		release, err := g.begin(write)
		if err != nil {
			t.Fatal(err)
		}
		release()
	}
	if len(c.slept) != 4 {
		t.Fatalf("slept %d times, want 4", len(c.slept))
	}
	want := writeGap + time.Duration(writeJitter/2*float64(writeGap))
	for _, d := range c.slept {
		if d != want {
			t.Errorf("a write waited %v, want %v", d, want)
		}
	}
}

// Reads are spaced by readGap: a page that loads a dozen lists goes out one
// call at a time, not as a burst.
func TestGateSpacesReads(t *testing.T) {
	g, c := newFakeGate()
	for range 6 {
		release, err := g.begin(read)
		if err != nil {
			t.Fatal(err)
		}
		release()
	}
	if len(c.slept) != 5 {
		t.Fatalf("slept %d times, want 5", len(c.slept))
	}
	for _, d := range c.slept {
		if d != readGap {
			t.Errorf("a read waited %v, want %v", d, readGap)
		}
	}
}

// A refusal of one change as risky (-460) stops the changes for a while,
// without calling NetEase again, and reads go on slower.
func TestGatePausesWritesAfter460(t *testing.T) {
	g, c := newFakeGate()
	g.refuse(-460)
	release, err := g.begin(write)
	if err == nil {
		release()
		t.Fatal("a write went out while the account was paused")
	}
	if !errors.Is(err, ErrPaused) {
		t.Fatalf("err = %v, want ErrPaused", err)
	}
	release, err = g.begin(read)
	if err != nil {
		t.Fatal(err)
	}
	release()
	if len(c.slept) != 0 {
		t.Fatalf("the first read after the refusal waited %v", c.slept[0])
	}
	release, err = g.begin(read)
	if err != nil {
		t.Fatal(err)
	}
	release()
	if len(c.slept) != 1 || c.slept[0] != pausedGap {
		t.Fatalf("reads while paused waited %v, want %v", c.slept, pausedGap)
	}
	c.at = c.at.Add(pause)
	if release, err := g.begin(write); err != nil {
		t.Fatalf("a write after the pause: %v", err)
	} else {
		release()
	}
}

// Other codes change nothing: a taken-down song says nothing of the account.
func TestGateIgnoresOtherCodes(t *testing.T) {
	g, _ := newFakeGate()
	g.refuse(404)
	if release, err := g.begin(write); err != nil {
		t.Fatalf("a write after a 404: %v", err)
	} else {
		release()
	}
}

// The calls that change the account go through the gate, those of the eapi
// and the web API alike: while it is paused, they give ErrPaused without
// calling NetEase.
func TestWritesGoThroughGate(t *testing.T) {
	old := calls
	calls = newGate()
	t.Cleanup(func() { calls = old })
	calls.refuse(429)
	for name, write := range map[string]func() error{
		"like":              func() error { return Like(1, true) },
		"like episode":      func() error { return LikeProgram(1, true) },
		"like comment":      func() error { return LikeComment("R_SO_4_1", 2, true) },
		"collect album":     func() error { return CollectAlbum(1, true) },
		"follow artist":     func() error { return FollowArtist(1, true) },
		"subscribe podcast": func() error { return SubscribePodcast(1, true) },
		"save playlist":     func() error { return SubscribePlaylist(1, true) },
		"create playlist":   func() error { _, err := CreatePlaylist("x"); return err },
		"rename playlist":   func() error { return RenamePlaylist(1, "x") },
		"add to list":       func() error { return AddToPlaylist(1, 2) },
		"remove from list":  func() error { return RemoveFromPlaylist(1, 2) },
		"delete playlist":   func() error { return DeletePlaylist(1) },
		"trash a song":      func() error { return FMTrash(1) },
	} {
		if err := write(); !errors.Is(err, ErrPaused) {
			t.Errorf("%s: err = %v, want ErrPaused", name, err)
		}
	}
}

// The session kept names the same device: the visitor ID, the login cookie
// and the site's token, sorted.
func TestSessionCookies(t *testing.T) {
	jar, _ := cookiejar.New(nil)
	old := httpClient.Jar
	httpClient.Jar = jar
	t.Cleanup(func() { httpClient.Jar = old })
	jar.SetCookies(siteURL, []*http.Cookie{
		{Name: "MUSIC_U", Value: "u", Path: "/"},
		{Name: "NMTID", Value: "n", Path: "/"},
		{Name: "__csrf", Value: "c", Path: "/"},
		{Name: "os", Value: "pc", Path: "/"},
	})
	if got, want := sessionCookies(), "MUSIC_U=u; NMTID=n; __csrf=c"; got != want {
		t.Errorf("session = %q, want %q", got, want)
	}
}

// One change goes at a time: the next waits until the one at NetEase is
// answered, though it is spaced by writeGap anyway.
func TestGateSerializesWrites(t *testing.T) {
	g, _ := newFakeGate()
	g.sleep = func(time.Duration) {}
	first, err := g.begin(write)
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	go func() {
		release, err := g.begin(write)
		if err == nil {
			release()
		}
		close(started)
	}()
	select {
	case <-started:
		t.Fatal("two changes went at once")
	case <-time.After(50 * time.Millisecond):
	}
	first()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("the next change did not go once the first was answered")
	}
}

// No more than inFlight calls wait on NetEase at once.
func TestGateCapsInFlight(t *testing.T) {
	g := newGate()
	g.sleep = func(time.Duration) {}
	holds := make([]func(), 0, inFlight)
	for range inFlight {
		release, err := g.begin(read)
		if err != nil {
			t.Fatal(err)
		}
		holds = append(holds, release)
	}
	taken := make(chan struct{})
	go func() {
		release, _ := g.begin(read)
		close(taken)
		release()
	}()
	select {
	case <-taken:
		t.Fatal("a call went out with inFlight calls at NetEase")
	case <-time.After(50 * time.Millisecond):
	}
	holds[0]()
	select {
	case <-taken:
	case <-time.After(time.Second):
		t.Fatal("a call did not go out once a slot was free")
	}
}
