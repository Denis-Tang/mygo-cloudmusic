package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"netease-music-mygo/internal/bilibili"
	"netease-music-mygo/internal/netease"
)

// A stretch is cut out of the video's sound by FFmpeg, with the seconds
// the tracklist gives and the song's name in its tags.
func TestCutBiliSliceAsksFFmpegForTheStretch(t *testing.T) {
	dir := t.TempDir()
	oldDir, oldRun := biliSlicesDir, runFFmpeg
	biliSlicesDir = func() (string, error) { return dir, nil }
	var args []string
	runFFmpeg = func(ctx context.Context, ffmpeg string, got []string) error {
		args = got
		return os.WriteFile(got[len(got)-1], []byte("slice"), 0o600)
	}
	t.Cleanup(func() { biliSlicesDir, runFFmpeg = oldDir, oldRun })

	it := biliItem{
		Track: netease.Track{
			Name: "歌二", Duration: 120000,
			Artists: []netease.Artist{{Name: "UP主"}},
			Album:   netease.Album{Name: "合集"},
		},
		Start: 60000,
		End:   180000,
	}
	path, err := cutBiliSlice(context.Background(), "ffmpeg", "src.m4a", it)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "歌二.m4a")
	if path != want {
		t.Fatalf("path %q, want %q", path, want)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"-ss 60.000", "-to 180.000", "-c copy", "title=歌二"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("ffmpeg args %q, want %q", joined, want)
		}
	}
	if _, err := cutBiliSlice(context.Background(), "ffmpeg", "src.m4a", it); err != nil {
		t.Fatal(err)
	}
	if got := args[len(args)-1]; got != filepath.Join(dir, "歌二 (2).m4a") {
		t.Fatalf("second cut names %q, want another file", got)
	}
}

// The sound of a video comes down once, and the file is kept for every
// stretch of it.
func TestBiliAudioFileKeepsTheSoundOnce(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Write([]byte("sound"))
	}))
	defer srv.Close()
	old := biliPlayURL
	biliPlayURL = func(context.Context, bilibili.Video) (bilibili.Audio, error) {
		return bilibili.Audio{URL: srv.URL, Headers: map[string]string{"Referer": "https://www.bilibili.com/"}}, nil
	}
	t.Cleanup(func() { biliPlayURL = old })

	dir := t.TempDir()
	v := bilibili.Video{Bvid: "BV1xx411c7mD", Aid: 42}
	path, err := biliAudioFile(context.Background(), dir, v)
	if err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(path); err != nil || string(b) != "sound" {
		t.Fatalf("file %q: %v", b, err)
	}
	again, err := biliAudioFile(context.Background(), dir, v)
	if err != nil || again != path || requests != 1 {
		t.Fatalf("again %q (%v), requests %d, want the kept file", again, err, requests)
	}
}

// A cut file takes the row's place, and plays as music of this machine.
func TestKeepBiliSliceFileReplacesTheStretch(t *testing.T) {
	playlistFile(t)
	a := newTestApp()
	a.settings = defaultSettings()
	dir := t.TempDir()
	oldDir := biliSlicesDir
	biliSlicesDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { biliSlicesDir = oldDir })

	v := bilibili.Video{Bvid: "BV1xx411c7mD", Aid: 42, Title: "合集", Duration: 10 * time.Minute}
	pl := a.makeBiliPlaylist(v, []biliCand{
		{Raw: "歌一", StartMs: 0},
		{Raw: "歌二", StartMs: 60000},
	}, false)
	pl.Collected = true
	a.biliPlaylists = []biliPlaylist{pl}

	path := filepath.Join(dir, "歌一.m4a")
	if err := os.WriteFile(path, []byte("audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	a.keepBiliSliceFile(pl.ID, "歌一", path)
	got := a.biliPlaylists[0].Items[0]
	if got.Start != 0 || got.End != 0 || got.Track.Name != "歌一" {
		t.Fatalf("row %+v, want it replaced by the file", got)
	}
	if _, ok := a.biliLocals[got.Track.ID]; !ok {
		t.Fatal("the cut file is not ready to play")
	}
	// The row of the other stretch is left as it was.
	if a.biliPlaylists[0].Items[1].End == 0 {
		t.Fatal("another stretch was replaced")
	}
}

func neteaseTrack(name, artist, album string, ms int64) (t struct {
	ID      int64
	Name    string
	Artists []struct {
		ID   int64
		Name string
	}
	Album     struct{ Name string }
	Duration  int64
	Privilege *struct{ Pl int }
}) {
	return
}
