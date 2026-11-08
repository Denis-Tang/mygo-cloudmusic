package player

import (
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/egoist/mygo/audio"

	"netease-music-mygo/internal/netease"
)

type playbackTransport func(*http.Request) (*http.Response, error)

func (f playbackTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Risk control and transport failures used to look like VIP restrictions and skip to another song.
func TestPlaybackFailureKeepsSelectedSong(t *testing.T) {
	old := http.DefaultTransport
	defer func() { http.DefaultTransport = old }()
	for _, tc := range []struct {
		name, body  string
		network     error
		audioStatus int
	}{
		{"risk control", `{"code":-460}`, nil, 0},
		{"missing stream", `{"code":200,"data":[]}`, nil, 0},
		{"signed out", `{"code":301}`, nil, 0},
		{"server error", `{"code":503}`, nil, 0},
		{"timeout", "", &net.DNSError{IsTimeout: true}, 0},
		{"DNS", "", &net.DNSError{IsNotFound: true}, 0},
		{"connection", "", errors.New("offline"), 0},
		{"audio denied", "", nil, 403},
		{"audio not found", "", nil, 404},
		{"decoder", "", nil, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				requests := 0
				http.DefaultTransport = playbackTransport(func(r *http.Request) (*http.Response, error) {
					if r.URL.Host == "example.com" {
						return &http.Response{StatusCode: tc.audioStatus, Body: io.NopCloser(strings.NewReader("not audio")), Header: http.Header{}}, nil
					}
					if !strings.Contains(r.URL.Path, "/song/enhance/player/url/") {
						return nil, errors.New("offline")
					}
					requests++
					if tc.network != nil {
						return nil, tc.network
					}
					body := tc.body
					if tc.audioStatus != 0 {
						body = `{"code":200,"data":[{"url":"https://example.com/full.mp3?sensitive-signature","type":"mp3","level":"exhigh"}]}`
					}
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
				})
				ch := make(chan func(), 32)
				p := New(func(fn func()) { ch <- fn })
				p.Cue([]netease.Track{{ID: 1}, {ID: 2, Fee: 1, Privilege: &netease.Privilege{Pl: 320000}}, {ID: 3}})
				p.Next()
				for p.Loading() {
					next(t, ch)
				}
				synctest.Wait()
				if requests != 1 || p.Current().ID != 2 || p.Playing() || p.Err() == "" || strings.Contains(p.Err(), "sensitive-signature") {
					t.Fatalf("requests %d, current %d, playing %v, error %q", requests, p.Current().ID, p.Playing(), p.Err())
				}
			})
		})
	}
}

// Selection stays on a failed song; advancing skips unavailable streams, not network errors.
func TestSelectedTrackDoesNotSkipOnFailure(t *testing.T) {
	old := http.DefaultTransport
	defer func() { http.DefaultTransport = old }()
	for _, unavailable := range []bool{false, true} {
		for _, action := range []string{"queue", "shuffle", "jump", "retry", "restore", "next"} {
			t.Run(fmt.Sprintf("%s/unavailable=%v", action, unavailable), func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					requests := 0
					http.DefaultTransport = playbackTransport(func(r *http.Request) (*http.Response, error) {
						if strings.Contains(r.URL.Path, "/song/enhance/player/url/") {
							requests++
							if unavailable && requests == 1 {
								return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"code":200,"data":[{"code":-110,"fee":1,"payed":0}]}`)), Header: http.Header{}}, nil
							}
						}
						return nil, errors.New("offline")
					})
					ch := make(chan func(), 32)
					p := New(func(fn func()) { ch <- fn })
					tracks := []netease.Track{{ID: 1}, {ID: 2}, {ID: 3}}
					switch action {
					case "queue", "shuffle":
						p.shuffle = action == "shuffle"
						p.PlayQueue(tracks, 1)
					case "jump":
						p.Cue(tracks)
						p.Jump(1)
					case "retry":
						p.Cue(tracks)
						p.at, p.track = 1, &p.queue[1]
						p.TogglePlay()
					case "restore":
						p.Restore(Session{Queue: tracks, Order: []int{0, 1, 2}, At: 1})
					case "next":
						p.Cue(tracks)
						p.Next()
					}
					for p.Loading() {
						next(t, ch)
					}
					synctest.Wait()
					wantID, wantRequests := int64(2), 1
					if action == "next" && unavailable {
						wantID, wantRequests = 3, 2
					}
					if p.Current().ID != wantID || requests != wantRequests || p.Playing() || p.Err() == "" {
						t.Fatalf("current %d, playing %v, error %q", p.Current().ID, p.Playing(), p.Err())
					}
				})
			})
		}
	}
}

// A filtered selection stops on that song; a radio exposes its scope before filtering.
func TestBlockedSelectionAndRadio(t *testing.T) {
	p := New(func(func()) {})
	tracks := []netease.Track{{ID: 1}, {ID: 2}, {ID: 3}}
	p.Blocked = func(netease.Track) bool { return true }
	p.PlayQueue(tracks, 1)
	if p.Current().ID != 2 || p.Playing() || p.Loading() || p.Err() == "" {
		t.Fatalf("filtered selection: %+v, playing %v, loading %v, error %q", p.Current(), p.Playing(), p.Loading(), p.Err())
	}
	p.Blocked = func(netease.Track) bool {
		if p.Radio() != "fm:DEFAULT" {
			t.Fatal("radio scope not set before filtering")
		}
		return true
	}
	p.PlayRadio("fm:DEFAULT", tracks, func(got func([]netease.Track)) { got(nil) })
	if p.Playing() || p.Loading() || p.Ready() || p.Err() == "" {
		t.Fatalf("filtered radio: playing %v, loading %v, ready %v, error %q", p.Playing(), p.Loading(), p.Ready(), p.Err())
	}
}

func TestPlayEmptyQueue(t *testing.T) {
	p := &Player{at: -1}
	p.PlayQueue(nil, 0) // an empty playlist's play button
	if p.Current() != nil {
		t.Error("playing from an empty queue")
	}
}

// silence is a long song of nothing.
type silence struct{}

func (silence) SampleRate() int               { return 48000 }
func (silence) Channels() int                 { return 2 }
func (silence) Len() int64                    { return 48000 * 600 }
func (silence) SetPosition(int64) error       { return nil }
func (silence) Read(b []float32) (int, error) { clear(b); return len(b), nil }

// posted collects what the player posts to the main thread, for the test
// to run there.
func posted() (chan func(), *Player) {
	ch := make(chan func(), 16)
	p := New(func(fn func()) { ch <- fn })
	p.Cue([]netease.Track{{ID: 1, Name: "一", Duration: 600000}})
	out := audio.NewPlayer(silence{})
	out.SetVolume(0)
	out.Play()
	p.Attach(out)
	p.fade.attach(out, 1)
	return ch, p
}

func next(t *testing.T, ch chan func()) {
	t.Helper()
	select {
	case fn := <-ch:
		fn()
	case <-time.After(2 * time.Second):
		t.Fatal("nothing posted")
	}
}

// Pausing fades out before the music stops; playing again fades it in.
func TestPauseFades(t *testing.T) {
	ch, p := posted()
	defer p.out.Close()
	p.TogglePlay()
	if p.Playing() || !p.out.Playing() {
		t.Fatal("paused at once, or not at all")
	}
	next(t, ch)
	if p.out.Playing() {
		t.Fatal("still playing after fading out")
	}
	if !p.fade.silent() {
		t.Error("faded out to something")
	}
	p.TogglePlay()
	if !p.Playing() || !p.out.Playing() {
		t.Fatal("did not play again")
	}
	time.Sleep(fadeIn + 50*time.Millisecond)
	p.fade.mu.Lock()
	level := p.fade.level(time.Now())
	p.fade.mu.Unlock()
	if level != 1 {
		t.Errorf("faded in to %v", level)
	}
}

// A seek playing pauses, which fades out over a few milliseconds, and
// plays again once there, its position the one asked for meanwhile.
func TestSeekWithoutClick(t *testing.T) {
	ch, p := posted()
	defer p.out.Close()
	p.Seek(90 * time.Second)
	if p.out.Playing() {
		t.Error("jumped while playing")
	}
	if p.Position() != 90*time.Second {
		t.Errorf("position %v while seeking", p.Position())
	}
	next(t, ch)
	if !p.out.Playing() || p.out.Position() < 90*time.Second {
		t.Errorf("after the seek: playing %v at %v", p.out.Playing(), p.out.Position())
	}
}

// Seeks one after the other, as the progress bar asks when let go, play
// on from the last.
func TestSeekTwicePlaysOn(t *testing.T) {
	ch, p := posted()
	defer p.out.Close()
	p.Seek(90 * time.Second)
	p.Seek(95 * time.Second)
	next(t, ch)
	for len(ch) > 0 {
		next(t, ch)
	}
	time.Sleep(seekGap * 2)
	for len(ch) > 0 {
		next(t, ch)
	}
	if !p.out.Playing() || p.out.Position() < 95*time.Second {
		t.Errorf("after two seeks: playing %v at %v", p.out.Playing(), p.out.Position())
	}
}

// A radio asks for more as its queue nears its end, and adds what is not
// in it already.
func TestRadioRefills(t *testing.T) {
	p := &Player{at: -1}
	p.Cue([]netease.Track{{ID: 1}, {ID: 2}})
	asked := 0
	p.AttachRadio("fm:DEFAULT", func(got func([]netease.Track)) {
		asked++
		got([]netease.Track{{ID: 2}, {ID: 3}})
	})
	if asked != 1 || len(p.Queue()) != 3 || p.Queue()[2].ID != 3 {
		t.Fatalf("asked %d, queue %v", asked, p.Queue())
	}
	if p.Radio() != "fm:DEFAULT" {
		t.Errorf("radio %q", p.Radio())
	}
	// Playing something else ends the radio.
	p.endRadio()
	if p.Radio() != "" || p.feed != nil {
		t.Error("radio still on")
	}
}

func TestNormGain(t *testing.T) {
	for _, c := range []struct {
		st   *netease.Stream
		want float64
	}{
		{nil, 1},
		{&netease.Stream{}, 1},
		{&netease.Stream{Gain: -6.6, Peak: 1}, math.Pow(10, -2.6/20)},
		// Quiet, it is lifted, but not past its peak.
		{&netease.Stream{Gain: 3, Peak: 0.8}, 1.25},
		{&netease.Stream{Gain: -4, Peak: 0}, 1},
	} {
		if g := normGain(c.st); math.Abs(g-c.want) > 1e-9 {
			t.Errorf("%+v: %v, want %v", c.st, g, c.want)
		}
	}
}

// The cache gives back what it kept, at the level asked for or a better
// one, and drops what was played longest ago once over its limit.
func TestCache(t *testing.T) {
	dir := t.TempDir()
	c := NewCache(dir, 400)
	store := func(id int64, st *netease.Stream, n int) {
		f := c.part(st)
		f.Write(make([]byte, n))
		f.Close()
		c.keep(id, st, f.Name(), int64(n))
	}
	store(1, &netease.Stream{Level: "lossless", Type: "flac", Gain: -3}, 100)
	dl, st, ok := c.lookup(1, "exhigh")
	if !ok || dl.size != 100 || st.Type != "flac" || st.Gain != -3 {
		t.Fatalf("lookup: %v %+v", ok, st)
	}
	dl.Close()
	if _, _, ok := c.lookup(1, "hires"); ok {
		t.Error("found a lossless song for Hi-Res")
	}
	old := time.Now().Add(-time.Hour)
	os.Chtimes(filepath.Join(dir, "1-lossless.flac"), old, old)
	store(2, &netease.Stream{Level: "exhigh", Type: "mp3"}, 100)
	store(3, &netease.Stream{Level: "exhigh", Type: "mp3"}, 100)
	store(4, &netease.Stream{Level: "exhigh", Type: "mp3"}, 100)
	store(5, &netease.Stream{Level: "exhigh", Type: "mp3"}, 100)
	if _, _, ok := c.lookup(1, "standard"); ok {
		t.Error("kept the song played longest ago")
	}
	if dl, _, ok := c.lookup(5, "standard"); !ok {
		t.Error("dropped the song just kept")
	} else {
		dl.Close()
	}
	if n := c.Size(); n > 400 {
		t.Errorf("takes %d of 400", n)
	}
	c.Clear()
	if n := c.Size(); n != 0 {
		t.Errorf("cleared to %d", n)
	}
}

// Leaving a radio keeps the song playing and those before it, drops those
// it queued after, and asks it for no more.
func TestEndRadio(t *testing.T) {
	p := &Player{at: -1}
	p.Cue([]netease.Track{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}})
	p.at, p.track = 1, &p.queue[1]
	asked := 0
	p.AttachRadio("heart", func(got func([]netease.Track)) { asked++ })
	p.EndRadio()
	if p.Radio() != "" || p.Current().ID != 2 {
		t.Fatalf("radio %q, current %v", p.Radio(), p.Current())
	}
	if q := p.Queue(); len(q) != 2 || q[0].ID != 1 || q[1].ID != 2 {
		t.Fatalf("queue %v", q)
	}
	p.refill()
	if asked != 0 {
		t.Error("asked the radio after leaving it")
	}
}
