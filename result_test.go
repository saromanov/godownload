package godownload

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// clean removes test artifacts upfront: leftover files from
// interrupted runs would otherwise fail the test
func clean(paths ...string) {
	for _, p := range paths {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			panic(err)
		}
	}
}

func TestDownloadResult(t *testing.T) {
	clean("file.zip")
	srv := newTestServer(t, http.StatusOK, testBody)
	gd := &GoDownload{Quiet: true}
	res, err := gd.DownloadResult(t.Context(), srv.URL+"/file.zip", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.URL != srv.URL+"/file.zip" {
		t.Fatalf("unexpected URL: %q", res.URL)
	}
	if res.Path != "file.zip" {
		t.Fatalf("unexpected path: %q", res.Path)
	}
	if res.Size != int64(len(testBody)) || res.Bytes != int64(len(testBody)) {
		t.Fatalf("unexpected size: %d, bytes: %d", res.Size, res.Bytes)
	}
	if res.Segments != 1 {
		t.Fatalf("expected 1 segment, got %d", res.Segments)
	}
	if res.ChecksumVerified {
		t.Fatal("checksum must not be verified without Options.Checksum")
	}
	if res.Duration <= 0 {
		t.Fatal("expected positive duration")
	}
	data, err := os.ReadFile(res.Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != testBody {
		t.Fatalf("unexpected body: %q", data)
	}
	remove(t, "file.zip")
}

func TestDownloadResultChecksumVerified(t *testing.T) {
	clean("file.zip")
	srv := newTestServer(t, http.StatusOK, testBody)
	sum := sha256Sum(t, testBody)
	gd := &GoDownload{Quiet: true}
	res, err := gd.DownloadResult(t.Context(), srv.URL+"/file.zip",
		&Options{Checksum: "sha256:" + sum})
	if err != nil {
		t.Fatal(err)
	}
	if !res.ChecksumVerified {
		t.Fatal("expected ChecksumVerified to be true")
	}
	if res.Bytes != int64(len(testBody)) {
		t.Fatalf("unexpected bytes: %d", res.Bytes)
	}
	remove(t, "file.zip")
}

func TestMultiSegmentResult(t *testing.T) {
	clean("multi.bin")
	// 4 MiB body, 4 segments
	body := strings.Repeat("abcdefgh", 512*1024)
	srv := newRangeServer(t, body)
	t.Cleanup(func() { _ = os.Remove("multi.bin") })

	gd := &GoDownload{Quiet: true}
	res, err := gd.DownloadResult(t.Context(), srv.URL, &Options{
		Outpath:  "multi.bin",
		Segments: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Segments != 4 {
		t.Fatalf("expected 4 segments, got %d", res.Segments)
	}
	if res.Bytes != int64(len(body)) {
		t.Fatalf("expected %d bytes transferred, got %d", len(body), res.Bytes)
	}
	if res.Size != int64(len(body)) {
		t.Fatalf("expected size %d, got %d", len(body), res.Size)
	}
	if res.Path != "multi.bin" {
		t.Fatalf("unexpected path: %q", res.Path)
	}
	remove(t, "multi.bin")
}

func TestArchiveResultPath(t *testing.T) {
	clean("result_data.txt", "result_data.txt.zip")
	srv := newTestServer(t, http.StatusOK, testBody)
	gd := &GoDownload{Quiet: true}
	res, err := gd.DownloadResult(t.Context(), srv.URL+"/data.txt",
		&Options{Outpath: "result_data.txt", Archive: "zip"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Path != "result_data.txt.zip" {
		t.Fatalf("expected archive path, got %q", res.Path)
	}
	if res.Size == 0 {
		t.Fatal("expected positive archive size")
	}
	if exist("result_data.txt") {
		t.Fatal("original file should be removed after archiving")
	}
	remove(t, "result_data.txt.zip")
}

func TestOnDoneCallback(t *testing.T) {
	clean("done.bin")
	srv := newTestServer(t, http.StatusOK, testBody)
	called := make(chan Result, 1)
	gd := &GoDownload{Quiet: true, OnDone: func(r Result) {
		called <- r
	}}
	if err := gd.Download(srv.URL+"/done.bin", &Options{Outpath: "done.bin"}); err != nil {
		t.Fatal(err)
	}
	select {
	case r := <-called:
		if r.Path != "done.bin" {
			t.Fatalf("unexpected path: %q", r.Path)
		}
		if r.Size != int64(len(testBody)) {
			t.Fatalf("unexpected size: %d", r.Size)
		}
	default:
		t.Fatal("OnDone was not called")
	}
	remove(t, "done.bin")
}

func TestOnDoneOverridesGlobal(t *testing.T) {
	clean("done2.bin")
	srv := newTestServer(t, http.StatusOK, testBody)
	localCalled := false
	gd := &GoDownload{Quiet: true, OnDone: func(Result) {
		t.Error("global OnDone must not be called when Options.OnDone is set")
	}}
	opts := &Options{Outpath: "done2.bin", OnDone: func(Result) {
		localCalled = true
	}}
	if err := gd.Download(srv.URL+"/done2.bin", opts); err != nil {
		t.Fatal(err)
	}
	if !localCalled {
		t.Fatal("local OnDone was not called")
	}
	remove(t, "done2.bin")
}

func TestDownloadManyResult(t *testing.T) {
	clean("many_a.txt", "many_b.txt")
	srv := newTestServer(t, http.StatusOK, testBody)
	gd := &GoDownload{Quiet: true}
	res, err := gd.DownloadManyResult(t.Context(), []*Options{
		{URL: srv.URL + "/a.txt", Outpath: "many_a.txt"},
		{URL: srv.URL + "/b.txt", Outpath: "many_b.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 {
		t.Fatalf("expected 2 results, got %d", len(res))
	}
	for i, want := range []string{"many_a.txt", "many_b.txt"} {
		if res[i].Path != want {
			t.Fatalf("result %d: unexpected path %q", i, res[i].Path)
		}
		if res[i].Size != int64(len(testBody)) {
			t.Fatalf("result %d: unexpected size %d", i, res[i].Size)
		}
	}
	remove(t, "many_a.txt")
	remove(t, "many_b.txt")
}

func TestDownloadManyResultPartial(t *testing.T) {
	clean("partial_ok.txt", "partial_missing.txt")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "missing") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, testBody)
	}))
	t.Cleanup(srv.Close)
	gd := &GoDownload{Quiet: true}
	res, err := gd.DownloadManyResult(t.Context(), []*Options{
		{URL: srv.URL + "/ok.txt", Outpath: "partial_ok.txt"},
		{URL: srv.URL + "/missing.txt", Outpath: "partial_missing.txt"},
	})
	if len(res) != 2 {
		t.Fatalf("expected 2 result slots, got %d", len(res))
	}
	if err == nil {
		t.Fatal("expected aggregated error")
	}
	if res[0].Path != "partial_ok.txt" || res[0].Size == 0 {
		t.Fatalf("successful download result lost: %+v (err: %v)", res[0], err)
	}
	if res[1].Size != 0 {
		t.Fatalf("failed download must produce zero result: %+v", res[1])
	}
	remove(t, "partial_ok.txt")
}

func TestPageResult(t *testing.T) {
	clean("page_index.html")
	srv := newTestServer(t, http.StatusOK, testBody)
	gd := &GoDownload{Quiet: true}
	res, err := gd.DownloadResult(t.Context(), srv.URL+"/index.html",
		&Options{Page: true, Outpath: "page_index.html"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Path != "page_index.html" {
		t.Fatalf("unexpected path: %q", res.Path)
	}
	if res.Size != int64(len(testBody)) {
		t.Fatalf("unexpected size: %d", res.Size)
	}
	remove(t, "page_index.html")
}

func TestManifestResult(t *testing.T) {
	clean("manifest_data.bin", "manifest_result.yaml")
	srv := newTestServer(t, http.StatusOK, testBody)
	sum := sha256Sum(t, testBody)
	manifest := "manifest_result.yaml"
	content := fmt.Sprintf(`
- url: %s/data.bin
  out: manifest_data.bin
  checksum: sha256:%s
`, srv.URL, sum)
	if err := os.WriteFile(manifest, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(manifest) })

	gd := &GoDownload{Quiet: true}
	res, err := gd.FromManifestResult(t.Context(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 {
		t.Fatalf("expected 1 result, got %d", len(res))
	}
	if res[0].Path != "manifest_data.bin" {
		t.Fatalf("unexpected path: %q", res[0].Path)
	}
	if !res[0].ChecksumVerified {
		t.Fatal("expected checksum verification")
	}
	remove(t, "manifest_data.bin")
}

// newRangeServer is a server supporting Range requests for
// multi-segment tests
func newRangeServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		_, _ = fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func sha256Sum(t *testing.T, data string) string {
	t.Helper()
	sum := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", sum)
}
