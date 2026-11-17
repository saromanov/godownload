package godownload

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newHFServer emulates the HuggingFace API for a repository
func newHFServer(t *testing.T, repo string, files map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/models/"+repo {
			w.WriteHeader(http.StatusOK)
			siblings := make([]map[string]string, 0, len(files))
			for name := range files {
				siblings = append(siblings, map[string]string{"rfilename": name})
			}
			if err := json.NewEncoder(w).Encode(map[string]any{"siblings": siblings}); err != nil {
				t.Errorf("encoding model info: %v", err)
			}
			return
		}
		prefix := "/" + repo + "/resolve/main/"
		if body, ok := files[strings.TrimPrefix(r.URL.Path, prefix)]; ok {
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, body)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestDownloadHF(t *testing.T) {
	_ = os.RemoveAll("hfmodel")
	repo := "owner/model"
	srv := newHFServer(t, repo, map[string]string{
		"config.json":              `{"model_type":"test"}`,
		"tokenizer/tokenizer.json": `{"tokens":[]}`,
		"weights.bin":              "weightsdata",
	})
	gd := &GoDownload{Quiet: true}
	res, err := gd.DownloadHFResult(t.Context(), repo, &Options{
		Outpath:  "hfmodel",
		HFAPIURL: srv.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 3 {
		t.Fatalf("expected 3 results, got %d", len(res))
	}
	for _, r := range res {
		if r.Size == 0 {
			t.Fatalf("file not downloaded: %+v", r)
		}
	}
	// Nested paths preserved
	if !exist(filepath.Join("hfmodel", "config.json")) {
		t.Fatal("config.json not found")
	}
	if !exist(filepath.Join("hfmodel", "tokenizer", "tokenizer.json")) {
		t.Fatal("nested tokenizer.json not found")
	}
	data, err := os.ReadFile(filepath.Join("hfmodel", "weights.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "weightsdata" {
		t.Fatalf("unexpected body: %q", data)
	}
}

func TestDownloadHFError(t *testing.T) {
	// Repository does not exist
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `{"error":"Repository not found"}`)
	}))
	t.Cleanup(srv.Close)
	gd := &GoDownload{Quiet: true}
	if _, err := gd.DownloadHFResult(t.Context(), "owner/missing", &Options{
		HFAPIURL: srv.URL,
	}); err == nil {
		t.Fatal("expected an error for a missing repository")
	}
}

func TestDownloadHFEmptyRepo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"siblings":[]}`)
	}))
	t.Cleanup(srv.Close)
	gd := &GoDownload{Quiet: true}
	if _, err := gd.DownloadHFResult(t.Context(), "owner/empty", &Options{
		HFAPIURL: srv.URL,
	}); err == nil {
		t.Fatal("expected an error for an empty repository")
	}
}

func TestDownloadHFManyErrorPartial(t *testing.T) {
	_ = os.RemoveAll("hfpartial")
	// One of the files is missing on the resolve endpoint
	repo := "owner/partial"
	files := map[string]string{
		"good.json": `{"ok":true}`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/models/"+repo {
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `{"siblings":[{"rfilename":"good.json"},{"rfilename":"bad.json"}]}`)
			return
		}
		prefix := "/" + repo + "/resolve/main/"
		name := strings.TrimPrefix(r.URL.Path, prefix)
		if body, ok := files[name]; ok {
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, body)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	gd := &GoDownload{Quiet: true}
	res, err := gd.DownloadHFResult(t.Context(), repo, &Options{
		Outpath:  "hfpartial",
		HFAPIURL: srv.URL,
	})
	if err == nil {
		t.Fatal("expected an aggregated error")
	}
	found := false
	for _, r := range res {
		if r.Path == filepath.Join("hfpartial", "good.json") && r.Size > 0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("successful file result lost: %+v", res)
	}
	t.Cleanup(func() { _ = os.RemoveAll("hfpartial") })
}

func TestDownloadHFSimple(t *testing.T) {
	_ = os.RemoveAll("hfsimple")
	repo := "owner/simple-model"
	srv := newHFServer(t, repo, map[string]string{"config.json": `{}`})
	gd := &GoDownload{Quiet: true}
	if err := gd.DownloadHF(repo, &Options{Outpath: "hfsimple", HFAPIURL: srv.URL}); err != nil {
		t.Fatal(err)
	}
	if !exist(filepath.Join("hfsimple", "config.json")) {
		t.Fatal("config.json not found")
	}
	_ = os.RemoveAll("hfsimple")
}

func TestDownloadHFContextDownload(t *testing.T) {
	_ = os.RemoveAll("hfctx")
	repo := "owner/model"
	srv := newHFServer(t, repo, map[string]string{"a.txt": "a"})
	gd := &GoDownload{Quiet: true}
	if err := gd.DownloadHFContext(t.Context(), repo, &Options{Outpath: "hfctx", HFAPIURL: srv.URL}); err != nil {
		t.Fatal(err)
	}
	if !exist(filepath.Join("hfctx", "a.txt")) {
		t.Fatal("a.txt not found")
	}
	_ = os.RemoveAll("hfctx")
}

func TestDownloadHFGlobalOutdir(t *testing.T) {
	_ = os.RemoveAll("hfoutdir")
	repo := "owner/model"
	srv := newHFServer(t, repo, map[string]string{"m.txt": "m"})
	gd := &GoDownload{Quiet: true, Outdir: "hfoutdir"}
	if err := gd.DownloadHF(repo, &Options{HFAPIURL: srv.URL}); err != nil {
		t.Fatal(err)
	}
	if !exist(filepath.Join("hfoutdir", "m.txt")) {
		t.Fatal("m.txt not found in the global outdir")
	}
	_ = os.RemoveAll("hfoutdir")
}

func TestDownloadHFEmptyRepoName(t *testing.T) {
	gd := &GoDownload{Quiet: true}
	if _, err := gd.DownloadHFResult(t.Context(), "", nil); err == nil {
		t.Fatal("expected an error for an empty repository name")
	}
}

func TestDownloadHFInvalidAuthFormat(t *testing.T) {
	repo := "owner/model"
	srv := newHFServer(t, repo, map[string]string{"f.txt": "f"})
	gd := &GoDownload{Quiet: true}
	if _, err := gd.DownloadHFResult(t.Context(), repo, &Options{HFAPIURL: srv.URL, Auth: "useronly"}); err == nil {
		t.Fatal("expected an error for auth without a password")
	}
}

func TestDownloadHFDecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "not json")
	}))
	t.Cleanup(srv.Close)
	gd := &GoDownload{Quiet: true}
	if _, err := gd.DownloadHFResult(t.Context(), "owner/model", &Options{HFAPIURL: srv.URL}); err == nil {
		t.Fatal("expected a decoding error")
	}
}

func TestHfFilesNilClient(t *testing.T) {
	repo := "owner/model"
	srv := newHFServer(t, repo, map[string]string{"x.bin": "x"})
	// A nil client falls back to http.DefaultClient
	files, err := hfFiles(t.Context(), nil, repo, &Options{HFAPIURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0] != "x.bin" {
		t.Fatalf("unexpected file list: %v", files)
	}
}

func TestHfFilesCancelledContext(t *testing.T) {
	repo := "owner/model"
	srv := newHFServer(t, repo, map[string]string{"x.bin": "x"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := hfFiles(ctx, nil, repo, &Options{HFAPIURL: srv.URL}); err == nil {
		t.Fatal("expected an error for a cancelled context")
	}
}
