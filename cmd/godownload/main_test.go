package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/saromanov/godownload/internal/clitest"
)

// cliBin builds the CLI binary once for all tests; when the test run
// collects coverage, the binary is instrumented and its execution is
// merged into the coverage profile
func cliBin(t *testing.T) clitest.Binary {
	t.Helper()
	return clitest.Build(t, ".")
}

// jsonReport mirrors the JSON report of the CLI
type jsonReport struct {
	Downloads []struct {
		URL  string `json:"url"`
		Path string `json:"path"`
		Size int64  `json:"size"`
	} `json:"downloads"`
	Error string `json:"error"`
}

func TestCLINoArgs(t *testing.T) {
	res := clitest.Run(t, cliBin(t))
	if res.Code != 2 {
		t.Fatalf("expected exit code 2, got %d (stderr: %s)", res.Code, res.Stderr)
	}
}

func TestCLIDownload(t *testing.T) {
	srv := clitest.NewServer(t, "cli body")
	res := clitest.Run(t, cliBin(t), srv.URL+"/cli.txt")
	if res.Code != 0 {
		t.Fatalf("download failed: %s", res.Stderr)
	}
	if got := res.ReadFile(t, "cli.txt"); got != "cli body" {
		t.Fatalf("unexpected content: %q", got)
	}
}

func TestCLIDownloadWithOut(t *testing.T) {
	srv := clitest.NewServer(t, "out body")
	res := clitest.Run(t, cliBin(t), "-out", "result.bin", srv.URL+"/any.bin")
	if res.Code != 0 {
		t.Fatalf("download failed: %s", res.Stderr)
	}
	if got := res.ReadFile(t, "result.bin"); got != "out body" {
		t.Fatalf("unexpected content: %q", got)
	}
}

func TestCLIDownloadMany(t *testing.T) {
	srv := clitest.NewServer(t, "many")
	res := clitest.Run(t, cliBin(t), srv.URL+"/a.txt", srv.URL+"/b.txt")
	if res.Code != 0 {
		t.Fatalf("download failed: %s", res.Stderr)
	}
	if !res.FileExists("a.txt") || !res.FileExists("b.txt") {
		t.Fatal("files not downloaded")
	}
}

func TestCLIDownloadError(t *testing.T) {
	// Port 1 is not served: connection refused
	res := clitest.Run(t, cliBin(t), "http://127.0.0.1:1/nope.txt")
	if res.Code != 1 {
		t.Fatalf("expected exit code 1, got %d", res.Code)
	}
	if res.FileExists("nope.txt") {
		t.Fatal("no file must be created on failure")
	}
}

func TestCLIJSONReport(t *testing.T) {
	srv := clitest.NewServer(t, "json body")
	res := clitest.Run(t, cliBin(t), "-json", srv.URL+"/j.txt")
	if res.Code != 0 {
		t.Fatalf("download failed: %s", res.Stderr)
	}
	var rep jsonReport
	if err := json.Unmarshal([]byte(res.Stdout), &rep); err != nil {
		t.Fatalf("parsing JSON report: %v\n%s", err, res.Stdout)
	}
	if len(rep.Downloads) != 1 {
		t.Fatalf("expected 1 download in the report, got %d", len(rep.Downloads))
	}
	if rep.Downloads[0].Path != "j.txt" || rep.Downloads[0].Size != int64(len("json body")) {
		t.Fatalf("unexpected download entry: %+v", rep.Downloads[0])
	}
}

func TestCLIJSONReportError(t *testing.T) {
	res := clitest.Run(t, cliBin(t), "-json", "http://127.0.0.1:1/x.txt")
	if res.Code != 1 {
		t.Fatalf("expected exit code 1, got %d", res.Code)
	}
	var rep jsonReport
	if err := json.Unmarshal([]byte(res.Stdout), &rep); err != nil {
		t.Fatalf("parsing JSON report: %v\n%s", err, res.Stdout)
	}
	if rep.Error == "" {
		t.Fatal("expected an error field in the JSON report")
	}
}

func TestCLIFromFile(t *testing.T) {
	srv := clitest.NewServer(t, "from file body")
	links := filepath.Join(t.TempDir(), "links.txt")
	if err := os.WriteFile(links, []byte(srv.URL+"/ff.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := clitest.Run(t, cliBin(t), "-from-file", links)
	if res.Code != 0 {
		t.Fatalf("download failed: %s", res.Stderr)
	}
	if got := res.ReadFile(t, "ff.txt"); got != "from file body" {
		t.Fatalf("unexpected content: %q", got)
	}
}

func TestCLIFromFileError(t *testing.T) {
	res := clitest.Run(t, cliBin(t), "-from-file", "/nonexistent/links.txt")
	if res.Code != 1 {
		t.Fatalf("expected exit code 1, got %d", res.Code)
	}
}

func TestCLIManifest(t *testing.T) {
	srv := clitest.NewServer(t, "manifest body")
	manifest := filepath.Join(t.TempDir(), "manifest.yaml")
	content := "- url: " + srv.URL + "/mm.txt\n  out: mm.txt\n"
	if err := os.WriteFile(manifest, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	res := clitest.Run(t, cliBin(t), "-manifest", manifest)
	if res.Code != 0 {
		t.Fatalf("download failed: %s", res.Stderr)
	}
	if got := res.ReadFile(t, "mm.txt"); got != "manifest body" {
		t.Fatalf("unexpected content: %q", got)
	}
}

func TestCLIManifestError(t *testing.T) {
	res := clitest.Run(t, cliBin(t), "-manifest", "/nonexistent/manifest.yaml")
	if res.Code != 1 {
		t.Fatalf("expected exit code 1, got %d", res.Code)
	}
}

func TestCLIJSONManifest(t *testing.T) {
	srv := clitest.NewServer(t, "json manifest body")
	manifest := filepath.Join(t.TempDir(), "manifest.yaml")
	content := "- url: " + srv.URL + "/jm.txt\n  out: jm.txt\n"
	if err := os.WriteFile(manifest, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	res := clitest.Run(t, cliBin(t), "-json", "-manifest", manifest)
	if res.Code != 0 {
		t.Fatalf("download failed: %s", res.Stderr)
	}
	var rep jsonReport
	if err := json.Unmarshal([]byte(res.Stdout), &rep); err != nil {
		t.Fatalf("parsing JSON report: %v\n%s", err, res.Stdout)
	}
	if len(rep.Downloads) != 1 || rep.Downloads[0].Path != "jm.txt" {
		t.Fatalf("unexpected report: %+v", rep)
	}
}

func TestCLIJSONFromFile(t *testing.T) {
	srv := clitest.NewServer(t, "json from file body")
	links := filepath.Join(t.TempDir(), "links.txt")
	if err := os.WriteFile(links, []byte(srv.URL+"/jf.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := clitest.Run(t, cliBin(t), "-json", "-from-file", links)
	if res.Code != 0 {
		t.Fatalf("download failed: %s", res.Stderr)
	}
	var rep jsonReport
	if err := json.Unmarshal([]byte(res.Stdout), &rep); err != nil {
		t.Fatalf("parsing JSON report: %v\n%s", err, res.Stdout)
	}
	if len(rep.Downloads) != 1 || rep.Downloads[0].Path != "jf.txt" {
		t.Fatalf("unexpected report: %+v", rep)
	}
}

func TestCLIJSONHF(t *testing.T) {
	repo := "owner/model"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/models/"+repo {
			_, _ = w.Write([]byte(`{"siblings":[{"rfilename":"jhf.txt"}]}`))
			return
		}
		if r.URL.Path == "/"+repo+"/resolve/main/jhf.txt" {
			_, _ = w.Write([]byte("json hf body"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	res := clitest.Run(t, cliBin(t), "-json", "-hf", repo, "-hf-api-url", srv.URL, "-out", "jhfout")
	if res.Code != 0 {
		t.Fatalf("download failed: %s", res.Stderr)
	}
	var rep jsonReport
	if err := json.Unmarshal([]byte(res.Stdout), &rep); err != nil {
		t.Fatalf("parsing JSON report: %v\n%s", err, res.Stdout)
	}
	if len(rep.Downloads) != 1 || rep.Downloads[0].Path != "jhfout/jhf.txt" {
		t.Fatalf("unexpected report: %+v", rep)
	}
}

func TestCLIHF(t *testing.T) {
	repo := "owner/model"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/models/"+repo {
			_, _ = w.Write([]byte(`{"siblings":[{"rfilename":"hf.txt"}]}`))
			return
		}
		if r.URL.Path == "/"+repo+"/resolve/main/hf.txt" {
			_, _ = w.Write([]byte("hf body"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	res := clitest.Run(t, cliBin(t), "-hf", repo, "-hf-api-url", srv.URL, "-out", "hfout")
	if res.Code != 0 {
		t.Fatalf("download failed: %s", res.Stderr)
	}
	if got := res.ReadFile(t, "hfout/hf.txt"); got != "hf body" {
		t.Fatalf("unexpected content: %q", got)
	}
}
