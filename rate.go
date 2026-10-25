package godownload

import (
	"io"
	"time"
)

// rateLimitedReader caps the read speed at bps bytes per second
// with a token-bucket that refills continuously
type rateLimitedReader struct {
	r      io.Reader
	bps    int64
	tokens float64
	last   time.Time
	init   bool
}

func (r *rateLimitedReader) Read(b []byte) (int, error) {
	if !r.init {
		r.last = time.Now()
		r.tokens = float64(r.bps) / 10 // small initial burst
		r.init = true
	}
	if len(b) == 0 {
		return 0, nil
	}
	// Cap the chunk size for smooth pacing
	maxchunk := r.bps / 10
	if maxchunk < 1 {
		maxchunk = 1
	}
	want := int64(len(b))
	if want > maxchunk {
		want = maxchunk
	}

	for {
		now := time.Now()
		r.tokens += now.Sub(r.last).Seconds() * float64(r.bps)
		r.last = now
		if r.tokens > float64(r.bps) {
			r.tokens = float64(r.bps)
		}
		if r.tokens >= float64(want) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	r.tokens -= float64(want)

	return r.r.Read(b[:want])
}
