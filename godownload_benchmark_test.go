package godownload

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// BenchmarkDownload measures a single-stream download of a 4 MiB body.
func BenchmarkDownload(b *testing.B) {
	body := strings.Repeat("x", 4*1024*1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		_, _ = fmt.Fprint(w, body)
	}))
	defer srv.Close()

	gd := &GoDownload{Quiet: true}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := gd.Download(srv.URL, &Options{Outpath: "bench.bin", Overwrite: true}); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	_ = os.Remove("bench.bin")
}

// BenchmarkDownloadMultiSegment measures a 4-segment download of a 16 MiB body.
func BenchmarkDownloadMultiSegment(b *testing.B) {
	body := strings.Repeat("abcdefgh", 2*1024*1024) // 16 MiB
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		if rh := r.Header.Get("Range"); rh != "" {
			var start, end int64
			if _, err := fmt.Sscanf(rh, "bytes=%d-%d", &start, &end); err == nil {
				w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(body)))
				w.WriteHeader(http.StatusPartialContent)
				_, _ = fmt.Fprint(w, body[start:end+1])
				return
			}
		}
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		_, _ = fmt.Fprint(w, body)
	}))
	defer srv.Close()

	gd := &GoDownload{Quiet: true}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := gd.Download(srv.URL, &Options{Outpath: "bench_seg.bin", Overwrite: true, Segments: 4}); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	_ = os.Remove("bench_seg.bin")
}
