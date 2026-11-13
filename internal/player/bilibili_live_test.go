package player

import (
	"context"
	"os"
	"testing"
	"time"

	"netease-music-mygo/internal/bilibili"
	"netease-music-mygo/internal/netease"
)

// A video of Bilibili plays: its audio track opens from the URL the API
// gives, with the headers it asks for, decodes and seeks.
func TestBilibiliAudioLive(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip("set LIVE to use Bilibili's API")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	videos, err := bilibili.Search(ctx, "音乐", 1)
	if err != nil || len(videos) == 0 {
		t.Fatalf("search: %v (%d)", err, len(videos))
	}
	v := videos[0]
	au, err := bilibili.PlayURL(ctx, v)
	if err != nil {
		t.Fatalf("playurl: %v", err)
	}
	p := &Player{}
	info, s, err := p.open(netease.Track{ID: -1, Duration: v.Duration.Milliseconds()}, "high", Local{}, false, Remote{URL: au.URL, Headers: au.Headers}, true)
	if err != nil {
		t.Fatalf("open %s: %v", v.Bvid, err)
	}
	defer s.Close()
	if info.Type != "m4a" && info.Type != "mp3" && info.Type != "flac" {
		t.Errorf("type %q", info.Type)
	}
	buf := make([]float32, 8192)
	var peak float32
	// Some videos open with silence: a few seconds in, it is a song.
	for range 60 {
		n, err := s.Read(buf)
		if n == 0 {
			t.Fatalf("read: %v", err)
		}
		for _, v := range buf[:n] {
			peak = max(peak, abs(v))
		}
	}
	if peak < 0.01 {
		t.Errorf("decoded silence (peak %v)", peak)
	}
}

func abs(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
