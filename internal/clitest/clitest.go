// Package clitest provides helpers for testing the example and CLI
// binaries of the module. The binaries are built with -cover and
// inherit GOCOVERDIR from the test run, so their coverage is merged
// into the coverage profile of `go test -cover ./...`
package clitest

import (
	"fmt"
	"hash/fnv"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// Binary is the path to a built CLI binary
type Binary string

// buildDir holds the built binaries for the whole test run: it must
// outlive a single test, so t.TempDir cannot be used here
var (
	buildDirOnce sync.Once
	buildDir     string
	mu           sync.Mutex
	built        = map[string]Binary{}
)

// fnv32 returns a short hash of dir for binary file names
func fnv32(dir string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(dir))
	return h.Sum32()
}

// RunResult holds the outcome of a single CLI run
type RunResult struct {
	// Stdout and Stderr capture the output of the run
	Stdout string
	Stderr string
	// Code is the exit code of the process
	Code int
	// Dir is the working directory of the run; downloaded files
	// land there
	Dir string
}

// subCoverDir returns the directory for subprocess coverage data.
// CLITEST_GOCOVERDIR takes precedence: it lets the CI collect the
// coverage of the spawned binaries in a directory that survives the
// test run. Without it the inherited GOCOVERDIR of the test run is
// used (go test -cover ./...)
func subCoverDir() string {
	if d := os.Getenv("CLITEST_GOCOVERDIR"); d != "" {
		return d
	}
	return os.Getenv("GOCOVERDIR")
}

// FileExists reports whether name exists in the run directory
func (r RunResult) FileExists(name string) bool {
	if _, err := os.Stat(r.Dir + "/" + name); err != nil {
		return false
	}
	return true
}

// ReadFile reads name from the run directory
func (r RunResult) ReadFile(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(r.Dir + "/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// Build builds the package at dir into a binary and caches it for
// the whole test run. When the test run collects coverage (GOCOVERDIR
// is set), the binary is built with -cover so its execution reports
// coverage into the same directory
func Build(t *testing.T, dir string) Binary {
	t.Helper()
	mu.Lock()
	if bin, ok := built[dir]; ok {
		mu.Unlock()
		return bin
	}
	mu.Unlock()

	buildDirOnce.Do(func() {
		d, err := os.MkdirTemp("", "clitest-*")
		if err != nil {
			t.Fatalf("creating build dir: %v", err)
			return
		}
		buildDir = d
	})
	bin := Binary(filepath.Join(buildDir, fmt.Sprintf("bin-%x", fnv32(dir))))
	args := []string{"build"}
	if subCoverDir() != "" {
		args = append(args, "-cover")
	}
	args = append(args, "-o", string(bin), dir)
	cmd := exec.Command("go", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building %s: %v\n%s", dir, err, out)
	}
	mu.Lock()
	built[dir] = bin
	mu.Unlock()
	return bin
}

// Run runs the binary in a fresh temporary directory, so the test
// does not depend on the working directory of the parent. When
// subprocess coverage is collected, the child writes its coverage
// data into subCoverDir
func Run(t *testing.T, bin Binary, args ...string) RunResult {
	t.Helper()
	cmd := exec.Command(string(bin), args...)
	cmd.Dir = t.TempDir()
	if d := subCoverDir(); d != "" {
		cmd.Env = append(os.Environ(), "GOCOVERDIR="+d)
	}
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("running %s: %v", bin, err)
	}
	return RunResult{Stdout: stdout.String(), Stderr: stderr.String(), Code: code, Dir: cmd.Dir}
}

// NewServer starts an HTTP server that responds with body to any
// request. The server is closed on test cleanup
func NewServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}
