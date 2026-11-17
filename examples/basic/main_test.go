package main

import (
	"testing"

	"github.com/saromanov/godownload/internal/clitest"
)

func TestBasicExampleNoArgs(t *testing.T) {
	bin := clitest.Build(t, ".")
	res := clitest.Run(t, bin)
	if res.Code != 2 {
		t.Fatalf("expected exit code 2, got %d (stderr: %s)", res.Code, res.Stderr)
	}
}

func TestBasicExampleDownload(t *testing.T) {
	srv := clitest.NewServer(t, "basic body")
	bin := clitest.Build(t, ".")
	res := clitest.Run(t, bin, srv.URL+"/basic.txt")
	if res.Code != 0 {
		t.Fatalf("download failed: %s", res.Stderr)
	}
	if got := res.ReadFile(t, "basic.txt"); got != "basic body" {
		t.Fatalf("unexpected content: %q", got)
	}
}

func TestBasicExampleDownloadError(t *testing.T) {
	// Port 1 is not served: connection refused
	bin := clitest.Build(t, ".")
	res := clitest.Run(t, bin, "http://127.0.0.1:1/basic.txt")
	if res.Code != 1 {
		t.Fatalf("expected exit code 1, got %d", res.Code)
	}
}
