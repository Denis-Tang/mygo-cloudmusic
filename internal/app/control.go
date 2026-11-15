package app

import (
	"bytes"
	"fmt"
	"image/png"
	"os"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/control"
	"netease-music-mygo/internal/diag"
	"netease-music-mygo/internal/netease"
)

// startControl serves the app to tests and to the developer, when
// MYGO_CONTROL is set: it can be driven without a mouse or a screen.
func (a *app) startControl() {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		diag.Errorf("control: %v", err)
		return
	}
	srv, err := control.Start(dir, &driver{a: a})
	if err != nil {
		diag.Errorf("control: %v", err)
		return
	}
	a.control = srv
}

// driver answers the control API, on the app's main thread where it touches
// the view.
type driver struct {
	a  *app
	tt *ui.Tester
}

// onMain runs fn where the app's state is, and waits for it.
func onMain(fn func() error) error {
	done := make(chan error, 1)
	mygo.RunOnMain(func() { done <- fn() })
	select {
	case err := <-done:
		return err
	case <-time.After(10 * time.Second):
		return fmt.Errorf("the app did not answer")
	}
}

// tester builds the view off screen, at the window's size, or the default.
func (d *driver) tester() *ui.Tester {
	if d.tt != nil {
		return d.tt
	}
	w, h := 1200, 800
	if cw, ch := d.a.win.ContentSize(); cw > 0 && ch > 0 {
		w, h = cw, ch
	}
	d.tt = ui.NewTester(d.a.view, w, h)
	d.tt.SetDark(d.a.pal.dark)
	return d.tt
}

func (d *driver) State() (any, error) {
	var out any
	err := onMain(func() error {
		out = d.state()
		return nil
	})
	return out, err
}

func (d *driver) state() map[string]any {
	a := d.a
	st := map[string]any{
		"page":       pageName(a.route.page),
		"pageId":     a.route.id,
		"query":      a.route.query,
		"nowPlaying": a.nowPlaying,
		"playing":    a.pl.Playing(),
		"loading":    a.pl.Loading(),
		"positionMs": a.pl.Position().Milliseconds(),
		"volume":     a.volume,
		"queue":      len(a.pl.Queue()),
		"locals":     len(a.locals),
		"bili":       len(a.biliTracks),
		"biliUsers":  len(a.biliUsers),
		"follows":    len(a.follows),
		"biliLists":  len(a.biliPlaylists),
		"feed":       len(a.biliFeed),
		"new":        a.biliNewNum,
		"biliQuery":  a.biliQuery,
		"liked":      len(a.liked),
		"errors":     diag.Recent(20),
		"crashed":    a.crashed,
		"log":        diag.Path(),
		"pid":        os.Getpid(),
	}
	if tr := a.pl.Current(); tr != nil {
		st["track"] = map[string]any{
			"id":         tr.ID,
			"name":       tr.Name,
			"artist":     tr.ArtistNames(),
			"durationMs": tr.Duration,
		}
	}
	if a.pl.Err() != "" {
		st["error"] = a.pl.Err()
	}
	if a.win != nil {
		w, h := a.win.ContentSize()
		st["window"] = map[string]any{"width": w, "height": h, "visible": a.win.IsVisible(), "focused": a.win.IsFocused()}
	}
	return st
}

func (d *driver) Click(label string) error {
	return onMain(func() error {
		tt := d.tester()
		tt.Frame()
		if err := tt.Click(label); err != nil {
			return err
		}
		tt.Frame()
		diag.Logf("control click %q", label)
		d.a.redraw()
		return nil
	})
}

func (d *driver) Type(text string) error {
	return onMain(func() error {
		tt := d.tester()
		tt.Type(text)
		tt.Frame()
		diag.Logf("control type %q", text)
		d.a.redraw()
		return nil
	})
}

func (d *driver) Key(name string) error {
	key, ok := keys[strings.ToLower(name)]
	if !ok {
		return fmt.Errorf("unknown key %q", name)
	}
	return onMain(func() error {
		tt := d.tester()
		tt.Key(0, key)
		tt.Frame()
		diag.Logf("control key %q", name)
		d.a.redraw()
		return nil
	})
}

var keys = map[string]ui.Key{
	"enter":     ui.KeyEnter,
	"escape":    ui.KeyEscape,
	"space":     ui.KeySpace,
	"tab":       ui.KeyTab,
	"backspace": ui.KeyBackspace,
	"up":        ui.KeyUp,
	"down":      ui.KeyDown,
	"left":      ui.KeyLeft,
	"right":     ui.KeyRight,
}

func (d *driver) Seek(seconds float64) error {
	return onMain(func() error {
		d.a.pl.Seek(time.Duration(seconds * float64(time.Second)))
		diag.Logf("control seek %.1fs", seconds)
		d.a.redraw()
		return nil
	})
}

func (d *driver) Open(page string) error {
	return onMain(func() error {
		switch page {
		case "home", "daily", "charts", "fm", "collected", "local", "bili", "search", "playlist":
		default:
			return fmt.Errorf("unknown page %q", page)
		}
		if page == "search" {
			d.a.openPage(route{page: pageSearch, query: d.a.query, kind: netease.SearchSongs})
		} else {
			d.a.openPage(route{page: pages[page]})
		}
		diag.Logf("control open %q", page)
		return nil
	})
}

var pages = map[string]pageKind{
	"home":      pageHome,
	"daily":     pageDaily,
	"charts":    pageCharts,
	"fm":        pageFM,
	"collected": pageCollected,
	"local":     pageLocal,
	"bili":      pageBili,
	"biliList":  pageBiliPlaylist,
	"search":    pageSearch,
	"playlist":  pagePlaylist,
}

func (d *driver) Play(index int) error {
	return onMain(func() error {
		var tracks []netease.Track
		switch d.a.route.page {
		case pageBili:
			tracks = d.a.biliTracks
		case pageLocal:
			tracks = d.a.locals
		default:
			tracks = d.a.pl.Queue()
		}
		if index < 0 || index >= len(tracks) {
			return fmt.Errorf("index %d of %d", index, len(tracks))
		}
		d.a.pl.PlayQueue(tracks, index)
		diag.Logf("control play %d", index)
		d.a.redraw()
		return nil
	})
}

func (d *driver) Search(query string) error {
	return onMain(func() error {
		if d.a.route.page == pageBili {
			d.a.loadBili(query)
		} else {
			d.a.search(query)
		}
		diag.Logf("control search %q", query)
		return nil
	})
}

func (d *driver) Screenshot(width, height int) ([]byte, error) {
	var out []byte
	err := onMain(func() error {
		tt := d.tester()
		if width > 0 && height > 0 {
			tt.SetSize(width, height)
		}
		tt.Frame()
		// Let what fades in finish: the art of a cover, the words of a page.
		tt.Advance(700 * time.Millisecond)
		tt.Frame()
		var buf bytes.Buffer
		if err := png.Encode(&buf, tt.Image()); err != nil {
			return err
		}
		out = buf.Bytes()
		return nil
	})
	return out, err
}

func (d *driver) Quit() {
	diag.Logf("control quit")
	mygo.App.Quit()
}

// pageName is the name of a page for the control API.
func pageName(p pageKind) string {
	for name, kind := range pages {
		if kind == p {
			return name
		}
	}
	return "other"
}
