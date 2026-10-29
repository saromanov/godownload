package godownload

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

// etagServer answers with a fixed body and ETag; it optionally
// honours If-None-Match with a 304 response
func etagServer(t *testing.T, body, etag string, honour304 bool) (*httptest.Server, *int, *int) {
	t.Helper()
	requests, notModified := new(int), new(int)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*requests++
		if honour304 && r.Header.Get("If-None-Match") == etag {
			*notModified++
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", etag)
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv, requests, notModified
}

func TestNoStaleCacheHit(t *testing.T) {
	clean("cache_hit.bin", "cache_hit.bin.gd-meta")
	srv, requests, notModified := etagServer(t, testBody, `"v1"`, true)

	gd := &GoDownload{Quiet: true}
	res, err := gd.DownloadResult(t.Context(), srv.URL, &Options{
		Outpath: "cache_hit.bin",
		NoStale: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.CacheHit {
		t.Fatal("first download must not be a cache hit")
	}
	if res.Bytes != int64(len(testBody)) {
		t.Fatalf("unexpected bytes: %d", res.Bytes)
	}

	// Second download: the server answers 304
	res, err = gd.DownloadResult(t.Context(), srv.URL, &Options{
		Outpath: "cache_hit.bin",
		NoStale: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.CacheHit {
		t.Fatalf("expected cache hit, got: %+v", res)
	}
	if res.Bytes != 0 {
		t.Fatalf("cache hit must transfer no bytes, got %d", res.Bytes)
	}
	if res.Size != int64(len(testBody)) {
		t.Fatalf("unexpected size: %d", res.Size)
	}
	if res.Duration <= 0 {
		t.Fatal("expected positive duration")
	}
	if *notModified == 0 {
		t.Fatal("expected a conditional request to the server")
	}
	if *requests != 2 {
		t.Fatalf("expected 2 requests, got %d", *requests)
	}
	// The file content stays untouched
	data, err := os.ReadFile("cache_hit.bin")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != testBody {
		t.Fatalf("unexpected body: %q", data)
	}
	remove(t, "cache_hit.bin")
	_ = os.Remove("cache_hit.bin.gd-meta")
}

func TestNoStaleETagChanged(t *testing.T) {
	clean("cache_changed.bin", "cache_changed.bin.gd-meta")
	version := "1"
	serverETag := fmt.Sprintf(`"v%s"`, version)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == serverETag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", serverETag)
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, testBody+"-"+version)
	}))
	t.Cleanup(srv.Close)

	gd := &GoDownload{Quiet: true}
	if _, err := gd.DownloadResult(t.Context(), srv.URL, &Options{
		Outpath: "cache_changed.bin",
		NoStale: true,
	}); err != nil {
		t.Fatal(err)
	}
	// The resource changes on the server between the downloads
	version = "2"
	serverETag = fmt.Sprintf(`"v%s"`, version)
	// The server reports a new ETag: the file is re-downloaded
	res, err := gd.DownloadResult(t.Context(), srv.URL, &Options{
		Outpath: "cache_changed.bin",
		NoStale: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.CacheHit {
		t.Fatal("expected a re-download after the ETag change")
	}
	if res.Bytes == 0 {
		t.Fatal("expected bytes to be transferred")
	}
	data, err := os.ReadFile("cache_changed.bin")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(data), "-2") {
		t.Fatalf("expected the fresh body, got %q", data)
	}
	remove(t, "cache_changed.bin")
	_ = os.Remove("cache_changed.bin.gd-meta")
}

func TestNoStaleWithoutValidators(t *testing.T) {
	clean("cache_nov.bin", "cache_nov.bin.gd-meta")
	// Server without ETag/Last-Modified
	srv := newTestServer(t, http.StatusOK, testBody)
	gd := &GoDownload{Quiet: true}
	for i := 0; i < 2; i++ {
		res, err := gd.DownloadResult(t.Context(), srv.URL, &Options{
			Outpath: "cache_nov.bin",
			NoStale: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if res.CacheHit {
			t.Fatal("expected a re-download: the server sends no validators")
		}
	}
	if exist("cache_nov.bin.gd-meta") {
		t.Fatal("no sidecar expected without validators")
	}
	remove(t, "cache_nov.bin")
}

func TestNoStaleOffByDefault(t *testing.T) {
	clean("cache_off.bin", "cache_off.bin.gd-meta")
	srv, requests, notModified := etagServer(t, testBody, `"v1"`, true)
	gd := &GoDownload{Quiet: true}
	for i := 0; i < 2; i++ {
		if _, err := gd.DownloadResult(t.Context(), srv.URL, &Options{
			Outpath:   "cache_off.bin",
			Overwrite: true,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if *notModified != 0 || *requests != 2 {
		t.Fatalf("without NoStale the server must not see validators: %d requests, %d 304s", *requests, *notModified)
	}
	if exist("cache_off.bin.gd-meta") {
		t.Fatal("no sidecar expected without NoStale")
	}
	remove(t, "cache_off.bin")
}

func TestNoStaleMultiSegment(t *testing.T) {
	clean("cache_multi.bin", "cache_multi.bin.gd-meta")
	// Large body for multi-segment
	body := strings.Repeat("abcdefgh", 512*1024)
	etag := `"big-1"`
	var mu sync.Mutex
	seenConditional := new(bool)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == etag {
			mu.Lock()
			*seenConditional = true
			mu.Unlock()
			w.WriteHeader(http.StatusNotModified)
			return
		}
		if rh := r.Header.Get("Range"); rh != "" {
			var start, end int64
			if strings.Contains(rh, "-") && !strings.HasSuffix(rh, "-") {
				_, _ = fmt.Sscanf(rh, "bytes=%d-%d", &start, &end)
			} else {
				_, _ = fmt.Sscanf(rh, "bytes=%d-", &start)
				end = int64(len(body)) - 1
			}
			if start > int64(len(body)) || end >= int64(len(body)) {
				w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
				return
			}
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(body)))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = fmt.Fprint(w, body[start:end+1])
			return
		}
		w.Header().Set("ETag", etag)
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		_, _ = fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)

	gd := &GoDownload{Quiet: true}
	if _, err := gd.DownloadResult(t.Context(), srv.URL, &Options{
		Outpath:  "cache_multi.bin",
		Segments: 4,
		NoStale:  true,
	}); err != nil {
		t.Fatal(err)
	}
	meta := readCacheMeta("cache_multi.bin.gd-meta")
	if meta == nil || meta.ETag == "" {
		t.Fatal("expected a sidecar with ETag after multi-segment download")
	}
	// Second download: probe answers 304
	res, err := gd.DownloadResult(t.Context(), srv.URL, &Options{
		Outpath:  "cache_multi.bin",
		Segments: 4,
		NoStale:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.CacheHit {
		t.Fatalf("expected a cache hit, got: %+v", res)
	}
	if res.Bytes != 0 || res.Size != int64(len(body)) {
		t.Fatalf("unexpected hit result: %+v", res)
	}
	mu.Lock()
	defer mu.Unlock()
	if !*seenConditional {
		t.Fatal("expected the probe to send If-None-Match")
	}
	remove(t, "cache_multi.bin")
	_ = os.Remove("cache_multi.bin.gd-meta")
}

func TestNoStaleMetadataCleanupOnNoValidators(t *testing.T) {
	clean("cache_drop.bin", "cache_drop.bin.gd-meta")
	// First server sends ETag, second does not
	srv1, _, _ := etagServer(t, testBody, `"v1"`, false)
	gd := &GoDownload{Quiet: true}
	if _, err := gd.DownloadResult(t.Context(), srv1.URL, &Options{
		Outpath: "cache_drop.bin",
		NoStale: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat("cache_drop.bin.gd-meta"); err != nil {
		t.Fatal("expected a sidecar after the first download")
	}
	// A download from a server without validators drops the sidecar
	srv2 := newTestServer(t, http.StatusOK, testBody)
	if _, err := gd.DownloadResult(t.Context(), srv2.URL, &Options{
		Outpath: "cache_drop.bin",
		NoStale: true,
	}); err != nil {
		t.Fatal(err)
	}
	if exist("cache_drop.bin.gd-meta") {
		t.Fatal("expected the sidecar to be dropped without validators")
	}
	remove(t, "cache_drop.bin")
}
