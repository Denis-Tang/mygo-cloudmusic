package player

import "math"

// MyGo mixes at 48 kHz. Convert before its cubic interpolator, which
// attenuates treble at fractional ratios and aliases when downsampling.
const playbackRate = 48000

type resampledStream struct {
	stream
	up, down int64
	half     int64
	weights  [][]float64
	buf      []float32 // interleaved stereo, starting at source frame base
	work     [4096]float32
	base     int64
	pos      int64 // next output frame
	err      error
}

func resample(s stream) stream {
	rate := int64(s.SampleRate())
	if rate == playbackRate {
		return s
	}
	a, b := rate, int64(playbackRate)
	for b != 0 {
		a, b = b, a%b
	}
	r := &resampledStream{stream: s, up: playbackRate / a, down: rate / a}
	step := math.Max(float64(rate)/playbackRate, 1)
	r.half = int64(math.Ceil(64 * step))
	cutoff := 0.49 / step
	r.weights = make([][]float64, r.up)
	for phase := range r.weights {
		w := make([]float64, 2*r.half)
		var sum float64
		for i := range w {
			x := float64(phase)/float64(r.up) - float64(int64(i)-r.half+1)
			if math.Abs(x) >= float64(r.half) {
				continue
			}
			sinc := 2 * cutoff
			if x != 0 {
				sinc = math.Sin(2*math.Pi*cutoff*x) / (math.Pi * x)
			}
			p := math.Pi * x / float64(r.half)
			w[i] = sinc * (0.42 + 0.5*math.Cos(p) + 0.08*math.Cos(2*p))
			sum += w[i]
		}
		for i := range w {
			w[i] /= sum
		}
		r.weights[phase] = w
	}
	return r
}

func (r *resampledStream) SampleRate() int { return playbackRate }

func (r *resampledStream) Len() int64 {
	if n := r.stream.Len(); n >= 0 {
		return (n*r.up + r.down - 1) / r.down
	}
	return -1
}

func (r *resampledStream) Read(dst []float32) (int, error) {
	n := 0
	for n+2 <= len(dst) {
		center := r.pos * r.down / r.up
		for r.err == nil && r.base+int64(len(r.buf)/2) <= center+r.half {
			got, err := r.stream.Read(r.work[:])
			r.buf = append(r.buf, r.work[:got]...)
			r.err = err
		}
		end := r.base + int64(len(r.buf)/2)
		if center >= end {
			return n, r.err
		}
		w := r.weights[r.pos*r.down%r.up]
		first := center - r.half + 1
		var left, right float64
		for j := max(first, r.base); j < min(center+r.half+1, end); j++ {
			i := (j - r.base) * 2
			weight := w[j-first]
			left += float64(r.buf[i]) * weight
			right += float64(r.buf[i+1]) * weight
		}
		dst[n], dst[n+1] = float32(left), float32(right)
		n += 2
		r.pos++
	}
	// Keep the preceding samples that the next filter window still needs.
	drop := min(max(r.pos*r.down/r.up-r.half+1-r.base, 0), int64(len(r.buf)/2))
	r.buf = r.buf[:copy(r.buf, r.buf[drop*2:])]
	r.base += drop
	return n, nil
}

func (r *resampledStream) SetPosition(frame int64) error {
	// Read the filter's history as well, so a seek sounds like uninterrupted
	// playback at the same position, without adding a delay to the timeline.
	start := max(frame*r.down/r.up-r.half+1, 0)
	if err := r.stream.SetPosition(start); err != nil {
		return err
	}
	r.base, r.pos, r.err = start, frame, nil
	r.buf = r.buf[:0]
	return nil
}
