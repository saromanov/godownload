package godownload

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"
)

// hfAPIBase is the default HuggingFace base URL; it can be
// overridden with Options.HFAPIURL (self-hosted deployments, tests)
const hfAPIBase = "https://huggingface.co"

// hfAPITimeout limits the model metadata request
const hfAPITimeout = 30 * time.Second

// DownloadHF downloads all files of a HuggingFace model repository
// (https://huggingface.co/<owner>/<name>). Nested paths inside the
// repository are preserved. The repository files are downloaded
// concurrently with the global options applied to every file
func (gd *GoDownload) DownloadHF(repo string, opt *Options) error {
	return gd.DownloadHFContext(context.Background(), repo, opt)
}

// DownloadHFContext is the context-aware version of DownloadHF
func (gd *GoDownload) DownloadHFContext(ctx context.Context, repo string, opt *Options) error {
	_, err := gd.DownloadHFResult(ctx, repo, opt)
	return err
}

// DownloadHFResult is the context-aware DownloadHF reporting results.
// On partial failure it returns the results of the successful downloads
// together with the aggregated error
func (gd *GoDownload) DownloadHFResult(ctx context.Context, repo string, opt *Options) ([]Result, error) {
	files, err := hfFiles(ctx, gd.client(), repo, opt)
	if err != nil {
		return nil, err
	}

	// Nested paths are already in the per-file outpaths: a copy of
	// the config without Outdir keeps them intact
	sub := *gd
	sub.Outdir = ""
	// NoStale works per file; the global setting is honoured
	// when the per-file one is not set
	eopt := *opt
	if !eopt.NoStale && gd.NoStale {
		eopt.NoStale = true
	}
	base := ""
	if eopt.Outpath != "" {
		base = eopt.Outpath
	} else if gd.Outdir != "" {
		base = gd.Outdir
	}
	opts := make([]*Options, 0, len(files))
	for _, file := range files {
		out := file
		if base != "" {
			out = path.Join(base, file)
		}
		opts = append(opts, &Options{
			URL:        hfResolveURL(baseURL(opt), repo, file),
			Outpath:    out,
			Overwrite:  eopt.Overwrite,
			Alwaysnew:  eopt.Alwaysnew,
			UserAgent:  eopt.UserAgent,
			Retry:      eopt.Retry,
			Auth:       eopt.Auth,
			OnProgress: eopt.OnProgress,
			RateLimit:  eopt.RateLimit,
			Segments:   eopt.Segments,
			OnDone:     eopt.OnDone,
			NoStale:    eopt.NoStale,
		})
	}
	return sub.downloadMany(ctx, opts)
}

// hfResolveURL builds the URL to download a repository file
func hfResolveURL(base, repo, file string) string {
	return fmt.Sprintf("%s/%s/resolve/main/%s", base, repo, file)
}

// baseURL returns the effective HuggingFace base URL
func baseURL(opt *Options) string {
	if opt != nil && opt.HFAPIURL != "" {
		return opt.HFAPIURL
	}
	return hfAPIBase
}

// hfFiles lists the files of a repository via the model API
func hfFiles(ctx context.Context, client *http.Client, repo string, opt *Options) ([]string, error) {
	if repo == "" {
		return nil, errors.New("huggingface: repository is required")
	}
	ctx, cancel := context.WithTimeout(ctx, hfAPITimeout)
	defer cancel()
	api := fmt.Sprintf("%s/api/models/%s", baseURL(opt), repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	if opt != nil && opt.Auth != "" {
		res := strings.SplitN(opt.Auth, ":", 2)
		if len(res) != 2 {
			return nil, errors.New("authentication must be in the format username:password")
		}
		req.SetBasicAuth(res[0], res[1])
	}
	if opt != nil && opt.UserAgent != "" {
		req.Header.Set("User-Agent", opt.UserAgent)
	}
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching model info: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("fetching model info: status %d from %s: %s", resp.StatusCode, api, string(body))
	}
	var model struct {
		Siblings []struct {
			Rfilename string `json:"rfilename"`
		} `json:"siblings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&model); err != nil {
		return nil, fmt.Errorf("decoding model info: %w", err)
	}
	if len(model.Siblings) == 0 {
		return nil, fmt.Errorf("repository %s has no files", repo)
	}
	files := make([]string, 0, len(model.Siblings))
	for _, s := range model.Siblings {
		if s.Rfilename != "" {
			files = append(files, s.Rfilename)
		}
	}
	return files, nil
}
