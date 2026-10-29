# godownload

[![CI](https://github.com/saromanov/godownload/actions/workflows/ci.yml/badge.svg)](https://github.com/saromanov/godownload/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/saromanov/godownload.svg)](https://pkg.go.dev/github.com/saromanov/godownload)
[![Go Report Card](https://goreportcard.com/badge/github.com/saromanov/godownload)](https://goreportcard.com/report/github.com/saromanov/godownload)

A Go library for downloading files over HTTP: streaming to disk, retries with exponential backoff, parallel downloads, basic authentication, zip archiving, resumable and multi-segment downloads.

## Install

```
go get github.com/saromanov/godownload
```

## Usage

Download a file (the name is taken from the URL):

```go
package main

import godownload "github.com/saromanov/godownload"

func main() {
    gd := godownload.GoDownload{}
    if err := gd.Download("http://arxiv.org/pdf/1206.5538v3.pdf", nil); err != nil {
        panic(err)
    }
}
```

With a specific output file:

```go
gd := godownload.GoDownload{}
err := gd.Download("http://arxiv.org/pdf/1206.5538v3.pdf", &godownload.Options{Outpath: "fun.pdf"})
```

Download multiple files in parallel:

```go
gd := godownload.GoDownload{}
err := gd.DownloadMany([]*godownload.Options{
    {URL: "https://example.com/a.pdf", Outpath: "a.pdf"},
    {URL: "https://example.com/b.pdf", Outpath: "b.pdf"},
})
```

Download by links listed in a file:

```go
gd := godownload.GoDownload{}
err := gd.FromFile("links.txt")
```

Download an HTML page:

```go
gd := godownload.GoDownload{}
err := gd.Download("https://example.com", &godownload.Options{Page: true, Outpath: "index.html"})
```

Custom timeout and retries:

```go
gd := godownload.GoDownload{Timeout: 30 * time.Second}
err := gd.Download("https://example.com/file.zip", &godownload.Options{Retry: 3})
```

## API

### GoDownload

Global settings (all optional):

| Field | Description |
|-------|-------------|
| `Overwrite` | Overwrite a file with the same name |
| `Alwaysnew` | Always create a new file (`file_1`, `file_2`, ...) |
| `UserAgent` | User-Agent for HTTP requests |
| `Retry` | Number of retry attempts on network errors and 5xx responses |
| `Auth` | Basic authentication in the `username:password` format |
| `Archive` | Archive format for the downloaded file (`zip`) |
| `Configpath` | Path to a YAML config |
| `Outdir` | Directory for downloaded files |
| `Timeout` | HTTP request timeout (no timeout by default) |
| `Logger` | Custom logger (`nil` — standard `log`) |
| `Quiet` | Disable all logging output |
| `HTTPClient` | Custom `http.Client` |
| `OnProgress` | Progress callback (see below) |
| `Checksum` | Checksum verification (see below) |
| `RateLimit` | Download speed cap in bytes per second |
| `Segments` | Number of parallel ranged segments |

### Options

Per-download settings (the same fields where applicable) plus:

| Field | Description |
|-------|-------------|
| `URL` | URL — needed only for `DownloadMany` |
| `Page` | Download an HTML page instead of a file |
| `Outpath` | Path of the downloaded file |

All methods return `error`. Sentinel errors: `ErrFileExists`, `ErrChecksumMismatch`, `ErrUnsupportedArchive`.

### Download results

Every download method has a `*Result` variant that reports the outcome:
`DownloadResult`, `DownloadManyResult`, `DownloadManySimpleResult`,
`FromFileResult`, `FromManifestResult` — all take `context.Context`
and return `Result{URL, Path, Size, Bytes, Segments, ChecksumVerified, Duration}`.
Alternatively, `GoDownload.OnDone` (or `Options.OnDone`) is called once
per successfully downloaded file:

```go
gd := godownload.GoDownload{}
res, err := gd.DownloadResult(ctx, "https://example.com/model.bin",
    &godownload.Options{Checksum: "sha256:...", Segments: 8})
if err != nil {
    return err
}
fmt.Printf("saved to %s (%d bytes, %d segments, verified: %v, took %s)\n",
    res.Path, res.Size, res.Segments, res.ChecksumVerified, res.Duration)
```

`DownloadManyResult` returns the results of successful downloads together
with an aggregated error on partial failure.

> **v2.0:** methods no longer terminate the process via `log.Fatal`/`panic` and return `error` instead. This is a breaking change compared to v1.

## Reliability features

Atomic writes, resume and checksum verification are always on for file downloads
(`Options.Checksum` is opt-in):

- The file lands under its final name only after a successful write (atomic rename from `file.part`)
- Interrupted downloads resume via `Range` requests (`file.part` is kept on failure)
- `ErrFileExists`, `ErrChecksumMismatch`, `ErrUnsupportedArchive` are checkable with `errors.Is`

Progress, throttling and multi-segment downloads:

```go
gd := godownload.GoDownload{Timeout: 30 * time.Second}
err := gd.Download("https://example.com/big.iso", &godownload.Options{
    Outpath:   "big.iso",
    Segments:  8,                // parallel ranged segments
    RateLimit: 10 * 1024 * 1024, // 10 MiB/s
    Checksum:  "sha256:...",     // verify before the file becomes visible
    OnProgress: func(transferred, total int64) {
        fmt.Printf("\r%d / %d", transferred, total)
    },
})
```

Declarative downloads from a YAML manifest with mirrors:

```yaml
- url: https://example.com/model.bin
  out: model.bin
  checksum: sha256:b5bb9d80...
  segments: 8
  mirrors:
    - https://mirror.example.com/model.bin
- url: https://example.com/data.csv
  out: data.csv
  retry: 5
```

```go
gd := godownload.GoDownload{}
if err := gd.FromManifest("downloads.yaml"); err != nil {
    // ...
}
```

## Context support

Every download method has a context-aware variant (`DownloadContext`,
`DownloadManyContext`, `DownloadManySimpleContext`, `FromFileContext`,
`FromManifestContext`). Context cancellation aborts in-flight HTTP requests.

## CLI

The repository ships a CLI built on top of the library (see the [release workflow](.github/workflows/release.yml) for binaries):

```
godownload [flags] URL...
godownload --manifest downloads.yaml
godownload --from-file links.txt --segments 8 --checksum sha256:...
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Security issues: [SECURITY.md](SECURITY.md).
