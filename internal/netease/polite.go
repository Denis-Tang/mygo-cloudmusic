package netease

import (
	"errors"
	"math/rand/v2"
	"sync"
	"time"
)

// NetEase watches how an account calls it: a burst, or many calls at once,
// is what its risk control answers with -460 (the network looks risky),
// and what it may ban an account for. Every call to the API goes through
// gate, which spaces them, and which stops what changes the account for a
// while once NetEase refused one.

const (
	// readGap is the least time between two calls.
	readGap = 80 * time.Millisecond
	// writeGap is the least time between two calls that change the
	// account, which risk control watches closest. A like waits no longer
	// than this for the app's answer.
	writeGap = 900 * time.Millisecond
	// writeJitter is how much more than writeGap a write may wait, as a
	// share of it: calls at even intervals look like a machine's.
	writeJitter = 0.4
	// inFlight is how many calls may wait on NetEase at once.
	inFlight = 4
	// pause is how long calls that change the account wait after NetEase
	// refused one as risky or too frequent.
	pause = 10 * time.Minute
	// pausedGap spaces the reads while the account is paused: slower, but
	// a page still loads.
	pausedGap = 300 * time.Millisecond
)

// ErrPaused is what a call that changes the account gives while NetEase
// has refused one: calling again would look worse.
var ErrPaused = errors.New("网易云暂时限制了账号操作，请 10 分钟后再试")

// op is what a call does: read, or change the account.
type op int

const (
	read op = iota
	write
)

type gate struct {
	slots chan struct{}
	// writing holds the change under way: NetEase sees one change of the
	// account at a time, as a listener makes them, never two at once.
	writing sync.Mutex

	mu    sync.Mutex
	next  time.Time // when the next call may go
	until time.Time // until when the account is paused

	now    func() time.Time
	sleep  func(time.Duration)
	jitter func() float64
	// signedIn reports whether a call carries a session, and with it the
	// account the spacing protects: tests replace it.
	signedIn func() bool
}

var calls = newGate()

func newGate() *gate {
	return &gate{slots: make(chan struct{}, inFlight), now: time.Now, sleep: time.Sleep, jitter: rand.Float64, signedIn: hasSession}
}

// hasSession reports whether the jar holds the listener's login cookie.
func hasSession() bool {
	if httpClient.Jar == nil {
		return false
	}
	for _, c := range httpClient.Jar.Cookies(siteURL) {
		if c.Name == "MUSIC_U" && c.Value != "" {
			return true
		}
	}
	return false
}

// begin waits until a call of o may go, and returns what lets the next call
// go once it is answered. A write while the account is paused gives
// ErrPaused without calling NetEase.
func (g *gate) begin(o op) (func(), error) {
	// What carries no session carries no account: it is capped, but not
	// spaced, as nobody is watched for it.
	if o == read && !g.signedIn() {
		g.slots <- struct{}{}
		return func() { <-g.slots }, nil
	}
	if o == write {
		g.writing.Lock()
	}
	g.mu.Lock()
	now := g.now()
	gap := readGap
	switch {
	case o == write && now.Before(g.until):
		g.mu.Unlock()
		g.writing.Unlock()
		return nil, ErrPaused
	case o == write:
		gap = writeGap + time.Duration(g.jitter()*writeJitter*float64(writeGap))
	case now.Before(g.until):
		gap = pausedGap
	}
	at := now
	if g.next.After(at) {
		at = g.next
	}
	g.next = at.Add(gap)
	g.mu.Unlock()
	if d := at.Sub(now); d > 0 {
		g.sleep(d)
	}
	g.slots <- struct{}{}
	return func() {
		<-g.slots
		if o == write {
			g.writing.Unlock()
		}
	}, nil
}

// refuse pauses what changes the account if code is NetEase refusing a
// call as risky (-460) or too frequent (429).
func (g *gate) refuse(code int) {
	if code != -460 && code != 429 {
		return
	}
	g.mu.Lock()
	if until := g.now().Add(pause); until.After(g.until) {
		g.until = until
	}
	g.mu.Unlock()
}
