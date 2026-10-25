package godownload

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

const testBody = "hello world"

func newTestServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.WriteHeader(status)
		_, _ = fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func createFileWithLinks(t *testing.T, filename string, urls ...string) {
	t.Helper()
	data := "This file contains interesting links!\n"
	for _, u := range urls {
		data += "First: " + u + "\n"
	}
	if err := os.WriteFile(filename, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(filename) })
}

func TestDownload(t *testing.T) {
	srv := newTestServer(t, http.StatusOK, testBody)
	gd := &GoDownload{Quiet: true}
	if err := gd.Download(srv.URL+"/file.zip", nil); err != nil {
		t.Fatal(err)
	}
	if !exist("file.zip") {
		t.Fatal("Downloaded file file.zip not found")
	}
	data, err := os.ReadFile("file.zip")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != testBody {
		t.Fatalf("unexpected body: %q", data)
	}
	remove(t, "file.zip")
}

func TestDownloadCustomOutpath(t *testing.T) {
	srv := newTestServer(t, http.StatusOK, testBody)
	gd := &GoDownload{Quiet: true}
	if err := gd.Download(srv.URL+"/file.zip", &Options{Outpath: "fun.pdf"}); err != nil {
		t.Fatal(err)
	}
	if !exist("fun.pdf") {
		t.Fatal("Downloaded file fun.pdf not found")
	}
	remove(t, "fun.pdf")
}

func TestDownloadOverwriteError(t *testing.T) {
	srv := newTestServer(t, http.StatusOK, testBody)
	gd := &GoDownload{Quiet: true}
	if err := gd.Download(srv.URL+"/dup.txt", nil); err != nil {
		t.Fatal(err)
	}
	if err := gd.Download(srv.URL+"/dup.txt", nil); err == nil {
		t.Fatal("expected error on existing file without Overwrite")
	}
	remove(t, "dup.txt")
}

func TestDownloadOverwrite(t *testing.T) {
	srv := newTestServer(t, http.StatusOK, testBody)
	gd := &GoDownload{Quiet: true}
	opts := &Options{Outpath: "ow.txt", Overwrite: true}
	if err := gd.Download(srv.URL, opts); err != nil {
		t.Fatal(err)
	}
	if err := gd.Download(srv.URL, opts); err != nil {
		t.Fatal(err)
	}
	remove(t, "ow.txt")
}

func TestDownloadAlwaysNew(t *testing.T) {
	srv := newTestServer(t, http.StatusOK, testBody)
	gd := &GoDownload{Quiet: true}
	opts := &Options{Outpath: "an.txt", Alwaysnew: true}
	if err := gd.Download(srv.URL, opts); err != nil {
		t.Fatal(err)
	}
	if err := gd.Download(srv.URL, opts); err != nil {
		t.Fatal(err)
	}
	if !exist("an_1.txt") {
		t.Fatal("Downloaded file an_1.txt not found")
	}
	remove(t, "an.txt")
	remove(t, "an_1.txt")
}

func TestDownloadHTTPError(t *testing.T) {
	srv := newTestServer(t, http.StatusNotFound, "not found")
	gd := &GoDownload{Quiet: true}
	err := gd.Download(srv.URL+"/missing.zip", nil)
	if err == nil {
		t.Fatal("expected error on 404")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDownloadRetryThenSuccess(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = fmt.Fprint(w, testBody)
	}))
	t.Cleanup(srv.Close)

	gd := &GoDownload{Quiet: true}
	if err := gd.Download(srv.URL+"/f.txt", &Options{Retry: 3}); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("expected 2 calls, got %d", calls.Load())
	}
	remove(t, "f.txt")
}

func TestDownloadUserAgent(t *testing.T) {
	var ua atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua.Store(r.Header.Get("User-Agent"))
		_, _ = fmt.Fprint(w, testBody)
	}))
	t.Cleanup(srv.Close)

	gd := &GoDownload{Quiet: true}
	if err := gd.Download(srv.URL+"/f.txt", &Options{UserAgent: "test-agent"}); err != nil {
		t.Fatal(err)
	}
	if got := ua.Load().(string); got != "test-agent" {
		t.Fatalf("unexpected user agent: %q", got)
	}
	remove(t, "f.txt")
}

func TestDownloadAuth(t *testing.T) {
	var user, pass string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, _ := r.BasicAuth()
		user, pass = username, password
		_, _ = fmt.Fprint(w, testBody)
	}))
	t.Cleanup(srv.Close)

	gd := &GoDownload{Quiet: true}
	if err := gd.Download(srv.URL+"/f.txt", &Options{Auth: "user:secret"}); err != nil {
		t.Fatal(err)
	}
	if user != "user" || pass != "secret" {
		t.Fatalf("unexpected auth: %q:%q", user, pass)
	}
	remove(t, "f.txt")
}

func TestDownloadOutdir(t *testing.T) {
	srv := newTestServer(t, http.StatusOK, testBody)
	gd := &GoDownload{Quiet: true, Outdir: "out_test"}
	if err := gd.Download(srv.URL+"/f.txt", nil); err != nil {
		t.Fatal(err)
	}
	if !exist(filepath.Join("out_test", "f.txt")) {
		t.Fatal("Downloaded file in outdir not found")
	}
	if err := os.RemoveAll("out_test"); err != nil {
		t.Fatal(err)
	}
}

func TestDownloadMany(t *testing.T) {
	srv := newTestServer(t, http.StatusOK, testBody)
	items := []*Options{
		{URL: srv.URL + "/first.txt", Outpath: "first.txt"},
		{URL: srv.URL + "/second.txt", Outpath: "second.txt"},
	}

	gd := &GoDownload{Quiet: true}
	if err := gd.DownloadMany(items); err != nil {
		t.Fatal(err)
	}
	if !exist("first.txt") || !exist("second.txt") {
		t.Fatal("Downloaded files not found")
	}
	remove(t, "first.txt")
	remove(t, "second.txt")
}

func TestDownloadManyError(t *testing.T) {
	srv := newTestServer(t, http.StatusOK, testBody)
	bad := newTestServer(t, http.StatusInternalServerError, "oops")
	items := []*Options{
		{URL: srv.URL + "/first.txt", Outpath: "first.txt"},
		{URL: bad.URL + "/second.txt", Outpath: "second.txt"},
	}

	gd := &GoDownload{Quiet: true}
	err := gd.DownloadMany(items)
	if err == nil {
		t.Fatal("expected aggregated error")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !exist("first.txt") {
		t.Fatal("first file should be downloaded")
	}
	remove(t, "first.txt")
}

func TestDownloadPage(t *testing.T) {
	srv := newTestServer(t, http.StatusOK, "<html>page</html>")
	gd := &GoDownload{Quiet: true}
	if err := gd.Download(srv.URL+"/page.html", &Options{Page: true, Outpath: "page_out.html"}); err != nil {
		t.Fatal(err)
	}
	if !exist("page_out.html") {
		t.Fatal("Downloaded page not found")
	}
	remove(t, "page_out.html")
}

func TestDownloadPageErrorWithoutOutpath(t *testing.T) {
	srv := newTestServer(t, http.StatusOK, "<html>page</html>")
	gd := &GoDownload{Quiet: true}
	// Для страницы без имени файла в URL и без Outpath ожидаем ошибку
	err := gd.Download(srv.URL, &Options{Page: true})
	if err == nil {
		t.Fatal("expected error without outpath")
	}
}

func TestArchiveData(t *testing.T) {
	srv := newTestServer(t, http.StatusOK, testBody)
	gd := &GoDownload{Quiet: true}
	if err := gd.Download(srv.URL+"/arch.txt", &Options{Outpath: "arch.txt", Archive: "zip"}); err != nil {
		t.Fatal(err)
	}
	if !exist("arch.txt.zip") {
		t.Fatal("arch.txt.zip not found")
	}
	if exist("arch.txt") {
		t.Fatal("original file should be removed after archiving")
	}
	remove(t, "arch.txt.zip")
}

func TestUnsupportedArchive(t *testing.T) {
	srv := newTestServer(t, http.StatusOK, testBody)
	gd := &GoDownload{Quiet: true}
	err := gd.Download(srv.URL+"/arch.tar", &Options{Outpath: "arch.tar", Archive: "tar"})
	if err == nil {
		t.Fatal("expected error for unsupported archive")
	}
	remove(t, "arch.tar")
}

func TestFromFile(t *testing.T) {
	srv := newTestServer(t, http.StatusOK, testBody)
	createFileWithLinks(t, "simple", srv.URL+"/first.txt", srv.URL+"/second.txt")

	gd := &GoDownload{Quiet: true}
	if err := gd.FromFile("simple"); err != nil {
		t.Fatal(err)
	}
	if !exist("first.txt") || !exist("second.txt") {
		t.Fatal("Downloaded files not found")
	}
	remove(t, "first.txt")
	remove(t, "second.txt")
}

func TestFromFileInvalidURL(t *testing.T) {
	createFileWithLinks(t, "simple_bad", "httpfoo", srvURLInvalid)
	urls, err := fromFile("simple_bad")
	if err != nil {
		t.Fatal(err)
	}
	if len(urls) != 0 {
		t.Fatalf("expected no valid urls, got %v", urls)
	}
}

const srvURLInvalid = "notaurl"

func TestGetFileNameFromURL(t *testing.T) {
	cases := []struct {
		url    string
		want   string
		hasErr bool
	}{
		{"https://example.com/path/file.zip", "file.zip", false},
		{"https://example.com", "", true},
		{"https://example.com/", "", true},
		{"://bad", "", true},
	}
	for _, tc := range cases {
		got, err := getFileNameFromURL(tc.url)
		if tc.hasErr && err == nil {
			t.Errorf("expected error for %s", tc.url)
		}
		if !tc.hasErr && (err != nil || got != tc.want) {
			t.Errorf("getFileNameFromURL(%s) = %q, %v; want %q", tc.url, got, err, tc.want)
		}
	}
}

func TestHasURL(t *testing.T) {
	if hasURL("httpfoo.com") {
		t.Error("httpfoo.com should not be a URL")
	}
	if !hasURL("https://example.com") {
		t.Error("https://example.com should be a URL")
	}
	if hasURL("example.com") {
		t.Error("example.com should not be a URL")
	}
}

func TestNextFreeName(t *testing.T) {
	f, _ := os.Create("nf.txt")
	_ = f.Close()
	got := nextFreeName("nf.txt")
	if got != "nf_1.txt" {
		t.Fatalf("expected nf_1.txt, got %s", got)
	}
	remove(t, "nf.txt")
}

func exist(path string) bool {
	return checkExist(path)
}

func remove(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
}
