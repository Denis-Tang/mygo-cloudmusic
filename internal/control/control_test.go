package control

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

type fakeDriver struct {
	clicked string
	typed   string
	page    string
	index   int
	seeks   float64
	shot    []byte
	quit    bool
}

func (f *fakeDriver) State() (any, error)             { return map[string]any{"page": "bili"}, nil }
func (f *fakeDriver) Click(label string) error        { f.clicked = label; return nil }
func (f *fakeDriver) Type(text string) error          { f.typed = text; return nil }
func (f *fakeDriver) Key(name string) error           { return errors.New("no key") }
func (f *fakeDriver) Seek(seconds float64) error      { f.seeks = seconds; return nil }
func (f *fakeDriver) Open(page string) error          { f.page = page; return nil }
func (f *fakeDriver) Play(index int) error            { f.index = index; return nil }
func (f *fakeDriver) Search(query string) error       { return nil }
func (f *fakeDriver) Screenshot(w, h int) ([]byte, error) { return f.shot, nil }
func (f *fakeDriver) Quit()                           { f.quit = true }

// The API answers only to its token, and does what it is asked.
func TestServesTheDriver(t *testing.T) {
	dir := t.TempDir()
	d := &fakeDriver{shot: []byte("png")}
	s, err := Start(dir, d)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	if _, err := os.Stat(filepath.Join(dir, "control.json")); err != nil {
		t.Fatalf("control.json: %v", err)
	}
	base := s.URL()
	res, err := http.Get(base + "/state")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("without a token: %d, want 403", res.StatusCode)
	}
	res.Body.Close()
	get := func(path string) *http.Response {
		t.Helper()
		req, _ := http.NewRequest("GET", base+path, nil)
		req.Header.Set("X-Control-Token", s.Token)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	var state map[string]any
	if err := json.NewDecoder(get("/state?x=1").Body).Decode(&state); err != nil || state["page"] != "bili" {
		t.Fatalf("state %v, %v", state, err)
	}
	if res := get("/click?label=" + "哔哩哔哩"); res.StatusCode != 200 {
		t.Fatalf("click: %d", res.StatusCode)
	}
	if d.clicked != "哔哩哔哩" {
		t.Fatalf("clicked %q", d.clicked)
	}
	get("/type?text=" + "晴天")
	if d.typed != "晴天" {
		t.Errorf("typed %q", d.typed)
	}
	get("/open?page=local")
	if d.page != "local" {
		t.Errorf("opened %q", d.page)
	}
	get("/play?index=3")
	if d.index != 3 {
		t.Errorf("played %d", d.index)
	}
	get("/seek?sec=42.5")
	if d.seeks != 42.5 {
		t.Errorf("seek %v", d.seeks)
	}
	res = get("/screenshot.png")
	if res.Header.Get("Content-Type") != "image/png" {
		t.Errorf("screenshot type %q", res.Header.Get("Content-Type"))
	}
	res = get("/quit")
	if res.StatusCode != 200 {
		t.Fatalf("quit: %d", res.StatusCode)
	}
}
