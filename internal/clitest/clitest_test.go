package clitest

import (
	"strings"
	"testing"
)

func TestBuildAndRunNoArgs(t *testing.T) {
	// The basic example exits with code 2 without arguments
	bin := Build(t, "../../examples/basic")
	res := Run(t, bin)
	if res.Code != 2 {
		t.Fatalf("expected exit code 2, got %d (stderr: %s)", res.Code, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "usage") {
		t.Fatalf("expected usage in stdout, got %q", res.Stdout)
	}
}

func TestRunDownload(t *testing.T) {
	srv := NewServer(t, "server body")
	bin := Build(t, "../../examples/basic")
	res := Run(t, bin, srv.URL+"/data.txt")
	if res.Code != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr: %s)", res.Code, res.Stderr)
	}
	if !res.FileExists("data.txt") {
		t.Fatal("data.txt not found in the run directory")
	}
	if got := res.ReadFile(t, "data.txt"); got != "server body" {
		t.Fatalf("unexpected content: %q", got)
	}
}

func TestRunCapturesStreamsAndCode(t *testing.T) {
	res := Run(t, Binary("/bin/sh"), "-c", "echo out; echo err 1>&2; exit 3")
	if res.Code != 3 {
		t.Fatalf("expected exit code 3, got %d", res.Code)
	}
	if !strings.Contains(res.Stdout, "out") {
		t.Fatalf("expected stdout, got %q", res.Stdout)
	}
	if !strings.Contains(res.Stderr, "err") {
		t.Fatalf("expected stderr, got %q", res.Stderr)
	}
}

func TestFileExistsMissing(t *testing.T) {
	res := RunResult{Dir: t.TempDir()}
	if res.FileExists("missing.txt") {
		t.Fatal("expected false for a missing file")
	}
}
