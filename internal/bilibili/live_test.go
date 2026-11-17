package bilibili

import (
	"context"
	"io"
	"net/http"
	"os"
	"testing"
)

// The live tests call B 站 itself, as the player will: run them with
// LIVE=1 and a network that reaches it.

func live(t *testing.T) context.Context {
	t.Helper()
	if os.Getenv("LIVE") == "" {
		t.Skip("LIVE=1 to call B 站")
	}
	return context.Background()
}

func TestLiveSearch(t *testing.T) {
	ctx := live(t)
	videos, err := Search(ctx, "周杰伦", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(videos) == 0 {
		t.Fatal("no results")
	}
	for i, v := range videos[:min(3, len(videos))] {
		t.Logf("%d. %s — %s (%s, %s)", i+1, v.Title, v.Up, v.Bvid, v.Duration)
	}
}

func TestLiveResolve(t *testing.T) {
	ctx := live(t)
	for _, input := range []string{
		"https://www.bilibili.com/video/BV1GJ411x7h7",
		"av80433022",
		"https://b23.tv/BV1GJ411x7h7",
	} {
		v, err := Resolve(ctx, input)
		if err != nil {
			t.Fatalf("%s: %v", input, err)
		}
		t.Logf("%s -> %s %s cid %d (%s, %s)", input, v.Bvid, v.Title, v.CID, v.Up, v.Duration)
	}
}

func TestLivePlayURL(t *testing.T) {
	ctx := live(t)
	v, err := Resolve(ctx, "https://www.bilibili.com/video/BV1GJ411x7h7")
	if err != nil {
		t.Fatal(err)
	}
	audio, err := PlayURL(ctx, v)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("audio: %s (%s, %s)", audio.Type, audio.Duration, audio.URL)
	// The CDN answers to the headers Audio carries: read the first 1KB.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, audio.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	for k, val := range audio.Headers {
		req.Header.Set(k, val)
	}
	res, err := httpClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	n, err := io.CopyN(io.Discard, res.Body, 1024)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		t.Fatal(err)
	}
	// The request asks from the start, so the CDN sends a 206 of it.
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusPartialContent {
		t.Fatalf("status %d", res.StatusCode)
	}
	t.Logf("first %d bytes stream: %s", n, res.Status)
}

func TestLiveRanking(t *testing.T) {
	ctx := live(t)
	videos, err := Ranking(ctx, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(videos) == 0 {
		t.Fatal("no videos")
	}
	for i, v := range videos[:min(3, len(videos))] {
		t.Logf("%d. %s — %s (%s, %s)", i+1, v.Title, v.Up, v.Bvid, v.Duration)
	}
}

func TestLiveSubtitles(t *testing.T) {
	ctx := live(t)
	v, err := Resolve(ctx, "https://www.bilibili.com/video/BV1Gf4y1y7wc")
	if err != nil {
		t.Fatal(err)
	}
	lrc, err := Subtitles(ctx, v)
	if err != nil {
		t.Fatal(err)
	}
	if lrc == "" {
		// B 站 hands out subtitle addresses only to a signed-in player
		// now, so nothing is not an error.
		t.Logf("no subtitles for %s without an account (expected)", v.Bvid)
		return
	}
	t.Logf("subtitles: %d bytes, first lines:\n%s", len(lrc), lrc[:min(len(lrc), 200)])
}
