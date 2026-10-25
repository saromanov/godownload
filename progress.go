package godownload

import "io"

// progressUpdateInterval defines how often the progress callback fires
const progressUpdateInterval = 64 * 1024

// progressReader wraps a reader and calls the progress callback
// every progressUpdateInterval bytes and once at EOF
type progressReader struct {
	r           io.Reader
	onProgress  func(transferred, total int64)
	total       int64
	transferred int64
	sinceNotify int64
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 {
		p.transferred += int64(n)
		p.sinceNotify += int64(n)
		if p.sinceNotify >= progressUpdateInterval || err != nil {
			p.onProgress(p.transferred, max64(p.total, 0))
			p.sinceNotify = 0
		}
	}
	return n, err
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
