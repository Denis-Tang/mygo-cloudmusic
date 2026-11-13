package player

import (
	"encoding/binary"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	"netease-music-mygo/internal/netease"

	aacm4a "github.com/tphakala/go-m4a/aacm4a"
	aacpcm "github.com/tphakala/go-aac/pcm"
)

// m4aFile makes an MP4 of AAC, a second of a tone, as the test's song.
func m4aFile(t *testing.T, rate int) []byte {
	t.Helper()
	pcm := make([]byte, rate*4)
	for i := 0; i < rate; i++ {
		v := uint16(int16(8000 * math.Sin(2*math.Pi*440*float64(i)/float64(rate))))
		binary.LittleEndian.PutUint16(pcm[i*4:], v)
		binary.LittleEndian.PutUint16(pcm[i*4+2:], v)
	}
	f, err := os.CreateTemp(t.TempDir(), "*.m4a")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cfg := aacpcm.Config{SampleRate: rate, BitDepth: 16, Channels: 2, Bitrate: 128000}
	if err := aacm4a.EncodeInterleaved(f, cfg, pcm); err != nil {
		t.Fatalf("encode: %v", err)
	}
	b, err := os.ReadFile(f.Name())
	if err != nil || len(b) == 0 {
		t.Fatalf("m4a: %d bytes, %v", len(b), err)
	}
	return b
}

// An MP4 of AAC is opened from its container, decodes to sound, and seeks.
func TestM4AStream(t *testing.T) {
	const rate = 44100
	b := m4aFile(t, rate)
	dl := downloaded(b)
	s, err := openM4AStream(dl)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	if s.SampleRate() != rate || s.Channels() != 2 {
		t.Fatalf("stream %d Hz, %d channels, want %d and 2", s.SampleRate(), s.Channels(), rate)
	}
	if n := s.Len(); n < rate*9/10 || n > rate*11/10 {
		t.Fatalf("length %d frames, want about %d", n, rate)
	}
	dst := make([]float32, 4096)
	n, err := s.Read(dst)
	if err != nil && err != io.EOF {
		t.Fatalf("read: %v", err)
	}
	if n == 0 {
		t.Fatal("read no samples")
	}
	loud := false
	for _, v := range dst[:n] {
		if math.Abs(float64(v)) > 0.05 {
			loud = true
			break
		}
	}
	if !loud {
		t.Error("decoded silence")
	}
	if err := s.SetPosition(rate / 2); err != nil {
		t.Fatalf("seek: %v", err)
	}
	if n, err := s.Read(dst); n == 0 {
		t.Fatalf("read after seek: %v", err)
	}
}

// openStream reads the container's brand and decodes rather than trusting
// the name, as the lossless levels and Bilibili's tracks both make MP4s.
func TestOpenStreamSniffsM4A(t *testing.T) {
	b := m4aFile(t, 44100)
	s, err := openStream(downloaded(b), 1000)
	if err != nil {
		t.Fatalf("openStream: %v", err)
	}
	defer s.Close()
	if _, ok := s.(*m4aStream); !ok {
		t.Fatalf("stream %T, want an m4aStream", s)
	}
}

// A track of another source plays from its URL, with the headers its
// source asks for, and its audio is read without a size in advance.
func TestOpenRemoteM4A(t *testing.T) {
	b := m4aFile(t, 44100)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Referer") != "https://www.bilibili.com" {
			t.Errorf("referer %q", r.Header.Get("Referer"))
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(b)))
		w.Write(b)
	}))
	defer srv.Close()
	p := &Player{}
	rem := Remote{URL: srv.URL + "/audio.m4s", Headers: map[string]string{"Referer": "https://www.bilibili.com"}}
	info, s, err := p.open(netease.Track{ID: -1 << 40, Duration: 1000}, "high", Local{}, false, rem, true)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	if info.Type != "m4a" {
		t.Errorf("type %q, want m4a for .m4s", info.Type)
	}
	buf := make([]float32, 2048)
	if n, err := s.Read(buf); n == 0 {
		t.Fatalf("read: %v", err)
	}
}
