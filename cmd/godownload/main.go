// Command godownload downloads files from URLs or YAML manifests.
//
// Usage:
//
//	godownload [flags] URL...            download each URL
//	godownload --manifest file.yaml      download per a YAML manifest
//	godownload --from-file links.txt     download URLs listed in a text file
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	godownload "github.com/saromanov/godownload"
)

func main() {
	var (
		manifest = flag.String("manifest", "", "path to a YAML manifest")
		fromfile = flag.String("from-file", "", "path to a text file with URLs")
		hfrepo   = flag.String("hf", "", "HuggingFace repository (owner/name) to download fully")
		hfapi    = flag.String("hf-api-url", "", "HuggingFace-compatible API base URL (default: https://huggingface.co)")
		outdir   = flag.String("outdir", "", "directory for downloaded files")
		out      = flag.String("out", "", "output path (only with a single URL)")
		retry    = flag.Int("retry", 0, "number of retries per download")
		segments = flag.Int("segments", 1, "parallel ranged segments per download")
		rate     = flag.Int64("rate", 0, "rate limit in bytes per second (0 = unlimited)")
		checksum = flag.String("checksum", "", "checksum in the <algorithm>:<hex> format")
		timeout  = flag.Duration("timeout", 0, "HTTP timeout (0 = none)")
		quiet    = flag.Bool("quiet", false, "suppress log output")
		nostale  = flag.Bool("no-stale", false, "skip downloads when the server reports the file unchanged")
		jsonout  = flag.Bool("json", false, "print a machine-readable JSON report of the results")
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
		NoStale:    *nostale,
		Configpath: os.Getenv("GODOWNLOAD_CONFIG"),
	}

	switch {
	case *manifest != "":
		if *jsonout {
			printJSONReport(report(func() ([]godownload.Result, error) {
				return gd.FromManifestResult(context.Background(), *manifest)
			}))
			return
		}
		if err := gd.FromManifest(*manifest); err != nil {
			fatal(err)
		}
	case *fromfile != "":
		if *jsonout {
			printJSONReport(report(func() ([]godownload.Result, error) {
				return gd.FromFileResult(context.Background(), *fromfile)
			}))
			return
		}
		if err := gd.FromFile(*fromfile); err != nil {
			fatal(err)
		}
	case *hfrepo != "":
		if *jsonout {
			printJSONReport(report(func() ([]godownload.Result, error) {
				return gd.DownloadHFResult(context.Background(), *hfrepo, &godownload.Options{Outpath: *out, HFAPIURL: *hfapi})
			}))
			return
		}
		if err := gd.DownloadHF(*hfrepo, &godownload.Options{Outpath: *out, HFAPIURL: *hfapi}); err != nil {
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
		if *jsonout {
			printJSONReport(report(func() ([]godownload.Result, error) {
				return gd.DownloadManyResult(context.Background(), items)
			}))
			return
		}
		if err := gd.DownloadMany(items); err != nil {
			fatal(err)
		}
	default:
		flag.Usage()
		os.Exit(2)
	}
}

// report runs a bulk download and captures the aggregated error
func report(fn func() ([]godownload.Result, error)) ([]godownload.Result, error) {
	return fn()
}

// printJSONReport prints the machine-readable report for agents;
// exits with code 1 on error after printing the report
func printJSONReport(results []godownload.Result, err error) {
	type jsonReport struct {
		Downloads []godownload.Result `json:"downloads"`
		Error     string              `json:"error,omitempty"`
	}
	rep := jsonReport{Downloads: results}
	if err != nil {
		rep.Error = err.Error()
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if encErr := encoder.Encode(rep); encErr != nil {
		fatal(encErr)
	}
	if err != nil {
		os.Exit(1)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "godownload:", err)
	os.Exit(1)
}
