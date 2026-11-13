package local

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	aacm4a "github.com/tphakala/go-m4a/aacm4a"
	aacpcm "github.com/tphakala/go-aac/pcm"
)

// The same file keeps its ID from one scan to the next, and has a negative
// one that NetEase's songs never do.
func TestIDsStayAcrossScans(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "song.mp3"), []byte("ID3\x03\x00\x00\x00\x00\x00\x00"))
	first := Open([]string{dir}, "")
	second := Open([]string{dir}, "")
	a, b := first.Tracks(), second.Tracks()
	if len(a) != 1 || len(b) != 1 {
		t.Fatalf("tracks: %d and %d, want 1 each", len(a), len(b))
	}
	if a[0].ID != b[0].ID {
		t.Fatalf("IDs differ between scans: %d and %d", a[0].ID, b[0].ID)
	}
	if a[0].ID >= 0 {
		t.Fatalf("ID %d is not negative", a[0].ID)
	}
	if a[0].Title != "song" {
		t.Fatalf("title %q, want the file's name", a[0].Title)
	}
}

// A song and its .lrc beside it are found together, whatever the case of the
// extension.
func TestLyricsBesideTheFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "track.flac"), []byte("fLaC"))
	writeFile(t, filepath.Join(dir, "track.LRC"), []byte("[00:01.00]hello"))
	got := Open([]string{dir}, "").Tracks()
	if len(got) != 1 {
		t.Fatalf("tracks: %d, want 1", len(got))
	}
	if _, err := os.Stat(got[0].LRC); err != nil {
		t.Fatalf("LRC %q, want the lyrics beside the song", got[0].LRC)
	}
}

// A picture beside the songs is the cover of a song without one in its tags,
// and a folder without pictures has none.
func TestCoverBesideTheFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.mp3"), []byte("ID3\x03\x00\x00\x00\x00\x00\x00"))
	writeFile(t, filepath.Join(dir, "cover.jpg"), []byte("jpeg"))
	got := Open([]string{dir}, "").Tracks()
	if len(got) != 1 || got[0].Cover != filepath.Join(dir, "cover.jpg") {
		t.Fatalf("cover %q, want cover.jpg beside the song", got[0].Cover)
	}
}

// A folder inside a folder is read, and a hidden one is not.
func TestSkipsHiddenFolders(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.mp3"), []byte("ID3\x03\x00\x00\x00\x00\x00\x00"))
	writeFile(t, filepath.Join(dir, "album", "b.mp3"), []byte("ID3\x03\x00\x00\x00\x00\x00\x00"))
	writeFile(t, filepath.Join(dir, ".trash", "c.mp3"), []byte("ID3\x03\x00\x00\x00\x00\x00\x00"))
	got := Open([]string{dir}, "").Tracks()
	if len(got) != 2 {
		t.Fatalf("tracks: %d, want the album's two songs", len(got))
	}
}

// An MP3's Xing header says how long it plays, and without one its constant
// bit rate divides the file.
func TestMP3Duration(t *testing.T) {
	// MPEG 1 Layer III, 128 kbps, 44.1 kHz, stereo.
	head := []byte{0xff, 0xfb, 0x90, 0x00}
	head = append(head, make([]byte, 32)...)
	head = append(head, []byte("Xing")...)
	head = append(head, 0, 0, 0, 1) // flags: frames present
	head = append(head, 0, 0, 0x03, 0xe8)
	if got, want := mp3Duration(head, 0), 1000*1152*time.Second/44100; got != want {
		t.Fatalf("with Xing: %v, want %v", got, want)
	}
	// No Xing: 128 kbps makes a 160,000-byte song 10 seconds long.
	if got, want := mp3Duration([]byte{0xff, 0xfb, 0x90, 0x00}, 160000), 10*time.Second; got != want {
		t.Fatalf("without Xing: %v, want %v", got, want)
	}
}

// A FLAC's STREAMINFO gives its samples and their rate.
func TestFLACDuration(t *testing.T) {
	b := make([]byte, 4+4+34)
	copy(b, "fLaC")
	b[4] = 0x00       // STREAMINFO, the last block
	b[5], b[6], b[7] = 0, 0, 34
	packed := uint64(44100)<<44 | 44100*10
	binary.BigEndian.PutUint64(b[8+10:8+18], packed)
	if got, want := flacDuration(b), 10*time.Second; got != want {
		t.Fatalf("duration: %v, want %v", got, want)
	}
}

// writeM4A makes an MP4 of a second of a tone, as a song of the test's.
func writeM4A(t *testing.T, path string, rate int) {
	t.Helper()
	pcm := make([]byte, rate*4)
	for i := 0; i < rate; i++ {
		v := uint16(int16(8000 * math.Sin(2*math.Pi*440*float64(i)/float64(rate))))
		binary.LittleEndian.PutUint16(pcm[i*4:], v)
		binary.LittleEndian.PutUint16(pcm[i*4+2:], v)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cfg := aacpcm.Config{SampleRate: rate, BitDepth: 16, Channels: 2, Bitrate: 128000}
	if err := aacm4a.EncodeInterleaved(f, cfg, pcm); err != nil {
		t.Fatalf("encode: %v", err)
	}
}

func writeFile(t *testing.T, path string, b []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// An .m4a of AAC is read as a song, its length from the MP4's header.
func TestReadsM4A(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "song.m4a")
	writeM4A(t, path, 44100)
	got := Open([]string{dir}, "").Tracks()
	if len(got) != 1 {
		t.Fatalf("tracks: %d, want the m4a", len(got))
	}
	if got[0].Duration < 900*time.Millisecond || got[0].Duration > 1100*time.Millisecond {
		t.Fatalf("duration %v, want about a second", got[0].Duration)
	}
}
