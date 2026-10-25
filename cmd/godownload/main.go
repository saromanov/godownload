// Command godownload downloads files from URLs or YAML manifests.
//
// Usage:
//
//	godownload [flags] URL...            download each URL
//	godownload --manifest file.yaml      download per a YAML manifest
//	godownload --from-file links.txt     download URLs listed in a text file
package main

import (
	"flag"
	"fmt"
	"os"

	godownload "github.com/saromanov/godownload"
)

func main() {
	var (
		manifest = flag.String("manifest", "", "path to a YAML manifest")
		fromfile = flag.String("from-file", "", "path to a text file with URLs")
		outdir   = flag.String("outdir", "", "directory for downloaded files")
		out      = flag.String("out", "", "output path (only with a single URL)")
		retry    = flag.Int("retry", 0, "number of retries per download")
		segments = flag.Int("segments", 1, "parallel ranged segments per download")
		rate     = flag.Int64("rate", 0, "rate limit in bytes per second (0 = unlimited)")
		checksum = flag.String("checksum", "", "checksum in the <algorithm>:<hex> format")
		timeout  = flag.Duration("timeout", 0, "HTTP timeout (0 = none)")
		quiet    = flag.Bool("quiet", false, "suppress log output")
	)
	flag.Usage = func() {
		_, _ = fmt.Fprintf(flag.CommandLine.Output(), "Usage: godownload [flags] URL...\n\nFlags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	gd := &godownload.GoDownload{
		Outdir:     *outdir,
		Retry:      *retry,
		Segments:   *segments,
		RateLimit:  *rate,
		Checksum:   *checksum,
		Timeout:    *timeout,
		Quiet:      *quiet,
		Configpath: os.Getenv("GODOWNLOAD_CONFIG"),
	}

	switch {
	case *manifest != "":
		if err := gd.FromManifest(*manifest); err != nil {
			fatal(err)
		}
	case *fromfile != "":
		if err := gd.FromFile(*fromfile); err != nil {
			fatal(err)
		}
	case flag.NArg() > 0:
		if flag.NArg() == 1 && *out != "" {
			if err := gd.Download(flag.Arg(0), &godownload.Options{Outpath: *out}); err != nil {
				fatal(err)
			}
			return
		}
		items := make([]*godownload.Options, 0, flag.NArg())
		for _, u := range flag.Args() {
			items = append(items, &godownload.Options{URL: u})
		}
		if err := gd.DownloadMany(items); err != nil {
			fatal(err)
		}
	default:
		flag.Usage()
		os.Exit(2)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "godownload:", err)
	os.Exit(1)
}
