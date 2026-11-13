// Package local reads the listener's own music folders: the tags, covers and
// .lrc lyrics of each file, and how long it plays. A track found there has a
// negative ID, which the pages and the player read as a file on this machine
// rather than a song of NetEase.
package local

import (
	"encoding/binary"
	"hash/fnv"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dhowden/tag"
)

// A Track is a file of the library with what its tags say of it. Duration is
// how long it plays, from its MP3 frames or FLAC header, or 0 if unknown.
type Track struct {
	ID       int64
	Path     string
	Title    string
	Artist   string
	Album    string
	Duration time.Duration
	Cover    string // an image kept in the library's cache, or beside the file
	LRC      string // the .lrc beside the file
}

// Library is what the folders hold, read once: Tracks and Track are for the
// main thread, and a scan of the folders makes a new Library.
type Library struct {
	folders  []string
	cacheDir string
	tracks   []Track
	byID     map[int64]Track
}

// Open reads the music under folders. Covers kept in tags are written into
// cacheDir, under covers, when not there already.
func Open(folders []string, cacheDir string) *Library {
	l := &Library{cacheDir: cacheDir, byID: map[int64]Track{}}
	seen := map[string]bool{}
	for _, f := range folders {
		f = filepath.Clean(f)
		if f == "." || seen[f] {
			continue
		}
		seen[f] = true
		l.folders = append(l.folders, f)
	}
	l.scan()
	return l
}

// Folders returns the folders read, in the order they were added.
func (l *Library) Folders() []string { return l.folders }

// Tracks returns every file found, by artist, album and title.
func (l *Library) Tracks() []Track { return l.tracks }

// Track returns the track of id, or false.
func (l *Library) Track(id int64) (Track, bool) {
	t, ok := l.byID[id]
	return t, ok
}

// scan walks the folders for the files it reads, skipping hidden ones and
// the system's own.
func (l *Library) scan() {
	for _, folder := range l.folders {
		filepath.WalkDir(folder, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if name := d.Name(); path != folder && (strings.HasPrefix(name, ".") || name == "$RECYCLE.BIN" || name == "System Volume Information") {
					return filepath.SkipDir
				}
				return nil
			}
			if !isAudio(path) {
				return nil
			}
			if t, ok := l.read(path); ok {
				l.byID[t.ID] = t
				l.tracks = append(l.tracks, t)
			}
			return nil
		})
	}
	sort.SliceStable(l.tracks, func(i, j int) bool {
		a, b := l.tracks[i], l.tracks[j]
		if a.Artist != b.Artist {
			return a.Artist < b.Artist
		}
		if a.Album != b.Album {
			return a.Album < b.Album
		}
		if a.Title != b.Title {
			return a.Title < b.Title
		}
		return a.Path < b.Path
	})
}

// isAudio reports whether path is a file the library reads.
func isAudio(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp3", ".flac":
		return true
	}
	return false
}

// read reads one file: its tags, how long it plays, its cover and lyrics.
func (l *Library) read(path string) (Track, bool) {
	f, err := os.Open(path)
	if err != nil {
		return Track{}, false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return Track{}, false
	}
	t := Track{ID: idOf(path), Path: path, Title: strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))}
	var picture *tag.Picture
	if m, err := tag.ReadFrom(f); err == nil {
		if s := strings.TrimSpace(m.Title()); s != "" {
			t.Title = s
		}
		t.Artist = strings.TrimSpace(m.Artist())
		t.Album = strings.TrimSpace(m.Album())
		picture = m.Picture()
	}
	t.Duration = duration(f, info.Size())
	t.Cover = l.cover(t.ID, path, picture)
	t.LRC = lyrics(path)
	return t, true
}

// idOf is the negative ID of the file at path, the same on every scan.
func idOf(path string) int64 {
	h := fnv.New64a()
	io.WriteString(h, strings.ToLower(filepath.Clean(path)))
	id := -int64(h.Sum64() & math.MaxInt64)
	if id == 0 {
		return -1
	}
	return id
}

// cover keeps the picture in the tags in the cache, or else names one beside
// the file, or returns "".
func (l *Library) cover(id int64, path string, picture *tag.Picture) string {
	if picture != nil && len(picture.Data) > 0 && l.cacheDir != "" {
		ext := ".jpg"
		if picture.MIMEType == "image/png" {
			ext = ".png"
		}
		dir := filepath.Join(l.cacheDir, "covers")
		if err := os.MkdirAll(dir, 0o700); err == nil {
			out := filepath.Join(dir, itoa(id)+ext)
			if _, err := os.Stat(out); err == nil {
				return out
			}
			if err := os.WriteFile(out, picture.Data, 0o600); err == nil {
				return out
			}
		}
	}
	base := filepath.Dir(path)
	entries, err := os.ReadDir(base)
	if err != nil {
		return ""
	}
	for _, name := range []string{"cover", "folder", "front"} {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			ext := strings.ToLower(filepath.Ext(e.Name()))
			if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
				continue
			}
			if strings.TrimSuffix(strings.ToLower(e.Name()), ext) == name {
				return filepath.Join(base, e.Name())
			}
		}
	}
	return ""
}

// lyrics returns the .lrc beside the file, whatever its case, or "".
func lyrics(path string) string {
	base := strings.TrimSuffix(path, filepath.Ext(path))
	if _, err := os.Stat(base + ".lrc"); err == nil {
		return base + ".lrc"
	}
	if _, err := os.Stat(base + ".LRC"); err == nil {
		return base + ".LRC"
	}
	return ""
}

// itoa writes an ID as its digits, without the minus.
func itoa(id int64) string {
	if id < 0 {
		id = -id
	}
	var b [20]byte
	i := len(b)
	for id > 0 {
		i--
		b[i] = byte('0' + id%10)
		id /= 10
	}
	if i == len(b) {
		return "0"
	}
	return string(b[i:])
}

// duration reads how long the file plays: an MP3's frames, or a FLAC's
// header, or 0 when neither says.
func duration(f *os.File, size int64) time.Duration {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return 0
	}
	head := make([]byte, 64<<10)
	n, _ := io.ReadFull(f, head)
	head = head[:n]
	if i := indexOf(head, []byte("fLaC")); i >= 0 {
		return flacDuration(head[i:])
	}
	if i := indexOf(head, []byte("ID3")); i == 0 && n >= 10 {
		// Skip the ID3 tag, whose size is syncsafe.
		tagLen := int64(head[6]&0x7f)<<21 | int64(head[7]&0x7f)<<14 | int64(head[8]&0x7f)<<7 | int64(head[9]&0x7f)
		head = head[min(int64(n), 10+tagLen):]
	}
	return mp3Duration(head, size)
}

// indexOf finds b in s, or -1.
func indexOf(s, b []byte) int {
	for i := 0; i+len(b) <= len(s); i++ {
		if string(s[i:i+len(b)]) == string(b) {
			return i
		}
	}
	return -1
}

// bitrates and rates are MPEG 1 Layer III's tables; MPEG 2 and 2.5 halve the
// rates and their Layer III bitrates are a table of their own.
var mp3Bitrates = [16]int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0}
var mp3Rates = [4]int{44100, 48000, 32000, 0}
var mp3BitratesV2 = [16]int{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0}

// mp3Duration reads the first frame of the audio: an MPEG 1/2/2.5 Layer III
// frame, with a Xing or Info header giving its frames, or else a constant
// bit rate that divides the file's length.
func mp3Duration(head []byte, size int64) time.Duration {
	for i := 0; i+4 <= len(head); i++ {
		if head[i] != 0xff || head[i+1]&0xe0 != 0xe0 {
			continue
		}
		version := (head[i+1] >> 3) & 3 // 3: MPEG 1, 2: MPEG 2, 0: MPEG 2.5
		layer := (head[i+1] >> 1) & 3   // 1: Layer III
		bitrateIdx := int(head[i+2] >> 4)
		rateIdx := int((head[i+2] >> 2) & 3)
		if version == 1 || layer != 1 || bitrateIdx == 0 || bitrateIdx == 15 || rateIdx == 3 {
			continue
		}
		rate := mp3Rates[rateIdx]
		bitrate := mp3Bitrates[bitrateIdx]
		samples := 1152
		if version != 3 {
			rate /= 2
			if version == 0 {
				rate /= 2
			}
			bitrate = mp3BitratesV2[bitrateIdx]
			samples = 576
		}
		if rate == 0 || bitrate == 0 {
			continue
		}
		// A Xing or Info header follows the side information.
		mono := head[i+3]>>6 == 3
		side := 32
		if version == 3 {
			if mono {
				side = 17
			}
		} else if mono {
			side = 9
		} else {
			side = 17
		}
		j := i + 4 + side
		if j+12 <= len(head) {
			if xing := string(head[j : j+4]); xing == "Xing" || xing == "Info" {
				flags := binary.BigEndian.Uint32(head[j+4 : j+8])
				if flags&1 != 0 {
					frames := int64(binary.BigEndian.Uint32(head[j+8 : j+12]))
					if frames > 0 {
						return time.Duration(frames) * time.Duration(samples) * time.Second / time.Duration(rate)
					}
				}
			}
		}
		// Constant bit rate: the bytes left, at this rate.
		audio := size
		if i == 0 && len(head) >= 10 && string(head[:3]) == "ID3" {
			audio -= int64(head[6]&0x7f)<<21 | int64(head[7]&0x7f)<<14 | int64(head[8]&0x7f)<<7 | int64(head[9]&0x7f)
		}
		if audio <= 0 {
			return 0
		}
		ms := audio * 8 * 1000 / int64(bitrate*1000)
		return time.Duration(ms) * time.Millisecond
	}
	return 0
}

// flacDuration reads a FLAC's STREAMINFO, which gives the samples and their
// rate.
func flacDuration(b []byte) time.Duration {
	if len(b) < 4+4+34 || string(b[:4]) != "fLaC" {
		return 0
	}
	// The first block follows the marker: a header of 4 bytes, its type in
	// the low 7 bits, 0 for STREAMINFO.
	if b[4]&0x7f != 0 {
		return 0
	}
	info := b[8 : 8+34]
	x := binary.BigEndian.Uint64(info[10:18])
	rate := int(x >> 44)
	total := int64(x & 0xFFFFFFFFF)
	if rate == 0 || total == 0 {
		return 0
	}
	return time.Duration(total) * time.Second / time.Duration(rate)
}
