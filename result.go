package godownload

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Result describes the outcome of a single completed download.
// It is returned by the *Result methods and passed to OnDone
type Result struct {
	// URL is the source the file was downloaded from.
	// For manifest and mirror downloads it is the URL that
	// actually responded successfully
	URL string

	// Path is the final path of the downloaded file.
	// For archived downloads this is the path of the archive
	Path string

	// Size is the size of the final file on disk.
	// 0 is reported when the size cannot be determined
	Size int64

	// Bytes is the number of bytes transferred over the network.
	// For resumed downloads it may be less than Size
	Bytes int64

	// Segments is the number of parallel segments used.
	// 1 means a single-stream download
	Segments int

	// ChecksumVerified is true when a checksum was requested
	// (Options.Checksum) and the downloaded file passed verification
	ChecksumVerified bool

	// CacheHit is true when the download was skipped because the
	// server reported the file unchanged since the last download
	// (Options.NoStale). Bytes is 0 and Size is the size of the
	// already existing file
	CacheHit bool

	// Duration is the wall time the download took
	Duration time.Duration
}

// FromManifestResult is the context-aware FromManifest reporting results.
// On partial failure it returns the results of the successful downloads
// together with the aggregated error
func (gd *GoDownload) FromManifestResult(ctx context.Context, manifestpath string) ([]Result, error) {
	opts, err := manifestOptions(manifestpath)
	if err != nil {
		return nil, err
	}
	return gd.downloadMany(ctx, opts)
}

// manyOptions builds per-item options for a list of URLs
func manyOptions(items []string) ([]*Options, error) {
	result := make([]*Options, 0, len(items))
	for _, item := range items {
		name, err := getFileNameFromURL(item)
		if err != nil {
			return nil, err
		}
		result = append(result, &Options{URL: item, Outpath: name})
	}
	return result, nil
}

// downloadMany downloads several files concurrently and collects
// the results. Errors of individual downloads are aggregated
func (gd *GoDownload) downloadMany(ctx context.Context, items []*Options) ([]Result, error) {
	errs := make([]error, len(items))
	results := make([]Result, len(items))
	var wg sync.WaitGroup
	for i, item := range items {
		wg.Add(1)
		go func(idx int, it *Options) {
			defer wg.Done()
			results[idx], errs[idx] = gd.download(ctx, it.URL, it)
		}(i, item)
	}
	wg.Wait()

	multi := make([]error, 0, len(items))
	for _, err := range errs {
		if err != nil {
			multi = append(multi, err)
		}
	}
	if len(multi) > 0 {
		return results, errors.Join(multi...)
	}
	return results, nil
}
