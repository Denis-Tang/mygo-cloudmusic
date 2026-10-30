package player

import (
	"io"
	"math"
	"os"
	"strconv"
	"testing"
)

type pcmStream struct {
	samples   []float32
	rate, pos int
	closed    bool
}

func (s *pcmStream) SampleRate() int                { return s.rate }
func (s *pcmStream) Channels() int                  { return 2 }
func (s *pcmStream) Len() int64                     { return int64(len(s.samples) / 2) }
func (s *pcmStream) SetPosition(frame int64) error  { s.pos = int(frame) * 2; return nil }
func (s *pcmStream) Close() error                   { s.closed = true; return nil }
func (s *pcmStream) source() (*download, *spectrum) { return nil, nil }
func (s *pcmStream) Read(dst []float32) (int, error) {
	n := copy(dst[:min(len(dst)/2*2, 514)], s.samples[s.pos:])
	s.pos += n
	if s.pos == len(s.samples) {
		return n, io.EOF
	}
	return n, nil
}

func readPCM(t *testing.T, s stream, size int) []float32 {
	t.Helper()
	buf := make([]float32, size)
	var out []float32
	for {
		n, err := s.Read(buf)
		out = append(out, buf[:n]...)
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			t.Fatal("decoder made no progress")
		}
	}
}

// Music keeps its treble and stereo separation, without the ultrasonic
// content that the old mixer folded down into the audible range.
func TestResampleQuality(t *testing.T) {
	for _, rate := range []int{44100, 96000, 192000} {
		t.Run(strconv.Itoa(rate), func(t *testing.T) {
			leftHz, rightHz := 1000.0, 30000.0
			if rate == 44100 {
				leftHz, rightHz = 18000, 20000
			}
			raw := &pcmStream{rate: rate, samples: make([]float32, rate/4*2)}
			for i := 0; i < len(raw.samples)/2; i++ {
				raw.samples[2*i] = float32(0.5 * math.Sin(2*math.Pi*leftHz*float64(i)/float64(rate)))
				raw.samples[2*i+1] = float32(0.5 * math.Sin(2*math.Pi*rightHz*float64(i)/float64(rate)))
			}
			s := resample(raw)
			out := readPCM(t, s, 4096)
			if s.SampleRate() != playbackRate || int64(len(out)/2) != s.Len() {
				t.Fatalf("rate %d, frames %d, want %d", s.SampleRate(), len(out)/2, s.Len())
			}
			for ch, freq := range []float64{leftHz, rightHz} {
				var sine, cosine, power float64
				const start, end = 2400, 9600
				for i := start; i < end; i++ {
					x := float64(out[2*i+ch])
					phase := 2 * math.Pi * freq * float64(i) / playbackRate
					sine += x * math.Sin(phase)
					cosine += x * math.Cos(phase)
					power += x * x
				}
				if freq > playbackRate/2 {
					rms := math.Sqrt(power / (end - start))
					t.Logf("%.0f Hz rejection %.2f dB", freq, 20*math.Log10(rms/(0.5/math.Sqrt2)))
					if rms > 1e-5 {
						t.Errorf("%.0f Hz aliases at RMS %.8f", freq, rms)
					}
				} else {
					amplitude := 2 * math.Hypot(sine, cosine) / (end - start)
					t.Logf("%.0f Hz gain %.4f dB", freq, 20*math.Log10(amplitude/0.5))
					if math.Abs(amplitude-0.5) > 0.001 {
						t.Errorf("%.0f Hz amplitude %.6f, want 0.5", freq, amplitude)
					}
				}
			}
			s.Close()
			if !raw.closed {
				t.Error("source was not closed")
			}
		})
	}
	raw := &pcmStream{rate: playbackRate}
	if resample(raw) != raw {
		t.Error("48 kHz source did not pass through")
	}
}

// Seeking keeps the filter's history and exact output position; playback
// reaches EOF without losing its last samples and can seek back afterwards.
func TestResampleSeekAndEnd(t *testing.T) {
	b, err := os.ReadFile("testdata/seek.flac")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := openFLACStream(downloaded(b))
	if err != nil {
		t.Fatal(err)
	}
	s := resample(raw)
	defer s.Close()
	whole := readPCM(t, s, 4096)
	if len(whole) != 5*playbackRate*2 {
		t.Fatalf("decoded %d frames, want %d", len(whole)/2, 5*playbackRate)
	}
	for _, frame := range []int64{0, 1, 2047, 2048, 77777, 239990, 240000, 0} {
		if err := s.SetPosition(frame); err != nil {
			t.Fatal(err)
		}
		buf := make([]float32, 513)
		n, err := s.Read(buf)
		want := whole[frame*2:]
		if n != min(512, len(want)) || (err != nil && err != io.EOF) {
			t.Fatalf("seek %d: %d samples, %v", frame, n, err)
		}
		if diff := maxDiff(buf[:n], want); diff != 0 {
			t.Errorf("seek %d: sample difference %g", frame, diff)
		}
	}
}
