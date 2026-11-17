package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/saromanov/godownload/internal/clitest"
)

func TestManifestExampleNoArgs(t *testing.T) {
	bin := clitest.Build(t, ".")
	res := clitest.Run(t, bin)
	if res.Code != 2 {
		t.Fatalf("expected exit code 2, got %d (stderr: %s)", res.Code, res.Stderr)
	}
}

func TestManifestExampleDownload(t *testing.T) {
	srv := clitest.NewServer(t, "manifest body")
	manifest := filepath.Join(t.TempDir(), "manifest.yaml")
	content := "- url: " + srv.URL + "/m.txt\n  out: m.txt\n"
	if err := os.WriteFile(manifest, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := clitest.Build(t, ".")
	res := clitest.Run(t, bin, manifest)
	if res.Code != 0 {
		t.Fatalf("download failed: %s", res.Stderr)
	}
	if got := res.ReadFile(t, "m.txt"); got != "manifest body" {
		t.Fatalf("unexpected content: %q", got)
	}
}

func TestManifestExampleDownloadError(t *testing.T) {
	bin := clitest.Build(t, ".")
	res := clitest.Run(t, bin, "/nonexistent/manifest.yaml")
	if res.Code != 1 {
		t.Fatalf("expected exit code 1, got %d", res.Code)
	}
}
