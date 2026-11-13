package player

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	aacm4a "github.com/tphakala/go-m4a/aacm4a"
)

// m4aStream decodes AAC in an MP4 or a fragmented MP4, as songs of NetEase's
// lossless levels and Bilibili's audio tracks come: the container says where
// each access unit is, and the decoder turns them into stereo PCM.
type m4aStream struct {
	dec  *aacm4a.Decoder
	dl   *download
	rate int
	ch   int
	// left holds the decoded PCM not yet given: interleaved S16.
	left    []byte
	frames  int64
	drop    func()
	closeMu sync.Once
}

// downloadSeeker reads a download as a file, for parsers that seek: what it
// asks for that isn't in yet, it waits for.
type downloadSeeker struct {
	dl      *download
	pos     int64
	started bool
}

func (f *downloadSeeker) Read(p []byte) (int, error) {
	r := downloadReader{d: f.dl, pos: f.pos}
	n, err := r.Read(p)
	f.pos = r.pos
	return n, err
}

// Seek moves within the song. Its end is where the size of the song says,
// which a download not told it cannot know.
func (f *downloadSeeker) Seek(off int64, whence int) (int64, error) {
	switch whence {
	case io.SeekCurrent:
		off += f.pos
	case io.SeekEnd:
		f.dl.mu.Lock()
		// The size comes with the response, or else with all of it.
		for f.dl.size <= 0 && !f.dl.done {
			f.dl.cond.Wait()
		}
		size := f.dl.size
		if size <= 0 {
			size = f.dl.n
		}
		err := f.dl.err
		f.dl.mu.Unlock()
		if size <= 0 {
			if err != nil {
				return 0, err
			}
			return 0, errors.New("player: the song's size is unknown")
		}
		off += size
	}
	if off < 0 {
		return 0, errors.New("player: seek before the start of the song")
	}
	f.pos = off
	return off, nil
}

// openM4AStream opens the MP4 that dl fetches, as its first bytes say.
func openM4AStream(dl *download) (*m4aStream, error) {
	f := &downloadSeeker{dl: dl}
	dec, info, err := aacm4a.NewSeekableDecoder(f)
	if err != nil {
		return nil, err
	}
	s := &m4aStream{dec: dec, dl: dl, rate: info.SampleRate, ch: info.Channels, drop: dl.Close}
	if s.rate == 0 {
		return nil, fmt.Errorf("player: the song says no sample rate")
	}
	s.frames = int64(info.Duration.Seconds() * float64(s.rate))
	if s.frames <= 0 && info.FrameCount > 0 {
		s.frames = int64(info.FrameCount) * 1024
	}
	return s, nil
}

func (s *m4aStream) SampleRate() int { return s.rate }

// Channels is always 2: a mono track is heard on both sides, as an MP3's is.
func (s *m4aStream) Channels() int { return 2 }

func (s *m4aStream) Len() int64 {
	if s.frames <= 0 {
		return -1
	}
	return s.frames
}

// Read decodes the next samples into dst, as stereo: the channels past the
// first two are left out, and one is heard on both sides.
func (s *m4aStream) Read(dst []float32) (int, error) {
	frames := len(dst) / 2
	if frames == 0 {
		return 0, nil
	}
	need := frames * 4
	if len(s.left) < need {
		buf := make([]byte, need)
		n := copy(buf, s.left)
		var err error
		for n < need {
			var got int
			got, err = s.dec.Read(buf[n:])
			n += got
			if err != nil || got == 0 {
				break
			}
		}
		s.left = buf[:n]
		if n < 4 {
			if err == nil {
				err = io.ErrUnexpectedEOF
			}
			return 0, err
		}
	}
	got := min(need, len(s.left)) / 4
	for i := 0; i < got; i++ {
		var l, r int16
		src := s.left[i*4:]
		switch s.ch {
		case 1:
			l = int16(binary.LittleEndian.Uint16(src))
			r = l
		default:
			l = int16(binary.LittleEndian.Uint16(src))
			r = int16(binary.LittleEndian.Uint16(src[2:]))
		}
		dst[i*2] = float32(l) / (1 << 15)
		dst[i*2+1] = float32(r) / (1 << 15)
	}
	s.left = s.left[got*4:]
	return got * 2, nil
}

// SetPosition moves to frame, counted from the start, pre-rolling the
// decoder so that the sound from there is the one a straight play makes.
func (s *m4aStream) SetPosition(frame int64) error {
	if s.frames > 0 && frame > s.frames {
		frame = s.frames
	}
	s.left = nil
	return s.dec.SeekTime(time.Duration(frame) * time.Second / time.Duration(s.rate))
}

func (s *m4aStream) Close() error {
	s.closeMu.Do(func() {
		if s.drop != nil {
			s.drop()
		}
	})
	return nil
}

// source returns the download decoded, and no spectrum: the visualizers
// leave a song they cannot measure without.
func (s *m4aStream) source() (*download, *spectrum) { return s.dl, nil }
