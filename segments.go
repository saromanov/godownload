package godownload

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
)

// minSegmentSize is the smallest split allowed per segment
const minSegmentSize = 1024 * 1024

// multiSegmentDownload splits the file into opt.Segments ranged
// parts, downloads them in parallel into separate temp files and
// assembles the result into outpath. It falls back to a plain
// single-stream download when the server does not advertise
// support for Range requests
func (gd *GoDownload) multiSegmentDownload(ctx context.Context, path string, opt *Options, outpath string, starttime time.Time, cond *condHeaders) (Result, error) {
	retry := opt.Retry
	useragent := defaultUserAgent
	if opt.UserAgent != "" {
		useragent = opt.UserAgent
	}

	// Probe: request the first byte to learn the size and Range support
	probe, err := downloadGeneral(ctx, gd.client(), retry, path, useragent, opt.Auth, nil, nil, cond)
	if err != nil {
		return Result{}, err
	}
	defer func() {
		if cerr := probe.Body.Close(); cerr != nil {
			gd.logf("closing probe body: %v", cerr)
		}
	}()
	// The server reports the local file unchanged: skip the transfer
	if probe.StatusCode == http.StatusNotModified {
		res := Result{
			URL:      path,
			Path:     outpath,
			Size:     fileSize(outpath),
			Segments: 1,
			CacheHit: true,
			Duration: time.Since(starttime),
		}
		if opt.OnDone != nil {
			opt.OnDone(res)
		}
		gd.logf("Skipping download of %s: file is up to date", outpath)
		return res, nil
	}
	if err := checkStatusCode(probe, path); err != nil {
		return Result{}, err
	}
	saveCacheMeta(path, outpath, opt.NoStale, probe)

	total := probe.ContentLength
	acceptRanges := probe.Header.Get("Accept-Ranges") == "bytes"
	if total <= 0 || !acceptRanges || total < minSegmentSize {
		gd.logf("Server %s does not support segmented download, falling back to single stream", path)
		body := io.Reader(probe.Body)
		if opt.RateLimit > 0 {
			body = &rateLimitedReader{r: body, bps: opt.RateLimit}
		}
		if opt.OnProgress != nil {
			body = &progressReader{r: body, onProgress: opt.OnProgress, total: max64(total, 0)}
		}
		transferred, err := gd.downloadToFile(body, outpath, nil, opt.Checksum)
		if err != nil {
			return Result{}, err
		}
		verified := opt.Checksum != ""
		gd.logf("Finish to download from %s in %s. Transferred bytes: %d", path,
			time.Since(starttime), transferred)
		finalpath, _, err := gd.postProcess(opt, outpath, verified)
		if err != nil {
			return Result{}, err
		}
		res := Result{
			URL:              path,
			Path:             finalpath,
			Size:             fileSize(finalpath),
			Bytes:            transferred,
			Segments:         1,
			ChecksumVerified: verified,
			Duration:         time.Since(starttime),
		}
		if opt.OnDone != nil {
			opt.OnDone(res)
		}
		return res, nil
	}

	segments := opt.Segments
	if int64(segments) > total/minSegmentSize {
		segments = int(total / minSegmentSize)
		if segments < 1 {
			segments = 1
		}
	}
	chunk := total / int64(segments)

	type segResult struct {
		idx  int
		path string
		err  error
	}
	results := make(chan segResult, segments)
	segPaths := make([]string, segments)
	var wg sync.WaitGroup
	sharedProgress := gd.progressCounter(opt, total)

	for i := 0; i < segments; i++ {
		start := int64(i) * chunk
		end := start + chunk - 1
		if i == segments-1 {
			end = total - 1 // last segment takes the remainder
		}
		segPath := fmt.Sprintf("%s.part%d", outpath, i)
		segPaths[i] = segPath

		wg.Add(1)
		go func(idx int, start, end int64, segPath string) {
			defer wg.Done()
			if err := gd.downloadSegment(ctx, path, opt, retry, useragent, start, end, segPath, sharedProgress); err != nil {
				results <- segResult{idx: idx, path: segPath, err: err}
				return
			}
			results <- segResult{idx: idx, path: segPath}
		}(i, start, end, segPath)
	}
	wg.Wait()
	close(results)

	var segErrs []error
	for r := range results {
		if r.err != nil {
			segErrs = append(segErrs, fmt.Errorf("segment %d: %w", r.idx, r.err))
		}
	}
	if len(segErrs) > 0 {
		// Clean up all segment files on failure
		for _, sp := range segPaths {
			_ = os.Remove(sp)
		}
		return Result{}, errors.Join(segErrs...)
	}

	// Assemble segments into the final file atomically
	if err := assembleSegments(segPaths, outpath); err != nil {
		return Result{}, err
	}
	gd.logf("Finish to download from %s in %s. Transferred bytes: %d (%d segments)",
		path, time.Since(starttime), total, segments)
	finalpath, verified, err := gd.postProcess(opt, outpath, false)
	if err != nil {
		return Result{}, err
	}
	res := Result{
		URL:              path,
		Path:             finalpath,
		Size:             fileSize(finalpath),
		Bytes:            total,
		Segments:         segments,
		ChecksumVerified: verified,
		Duration:         time.Since(starttime),
	}
	if opt.OnDone != nil {
		opt.OnDone(res)
	}
	return res, nil
}

// downloadSegment fetches one ranged part of the file into segPath
func (gd *GoDownload) downloadSegment(ctx context.Context, path string, opt *Options, retry int, useragent string, start, end int64, segPath string, progress func(int64)) error {
	startpos := start
	// Resume the segment if a partial file exists
	if info, err := os.Stat(segPath); err == nil {
		startpos = start + info.Size()
		gd.logf("Resuming segment %s from byte %d", segPath, startpos)
	}
	if startpos > end {
		return nil // segment already complete
	}

	resp, err := downloadGeneral(ctx, gd.client(), retry, path, useragent, opt.Auth, &startpos, &end, nil)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			gd.logf("closing segment body: %v", cerr)
		}
	}()
	if err := checkStatusCode(resp, path); err != nil {
		return err
	}
	if resp.StatusCode != http.StatusPartialContent {
		drainBody(resp.Body)
		return ErrNoRangeSupport
	}

	flags := os.O_WRONLY | os.O_CREATE
	resumed := startpos > start
	if resumed {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}
	f, err := os.OpenFile(segPath, flags, 0o644)
	if err != nil {
		return fmt.Errorf("opening segment file: %w", err)
	}

	var reader io.Reader = resp.Body
	if progress != nil {
		reader = &countingReader{r: reader, onRead: progress}
	}
	if opt.RateLimit > 0 {
		reader = &rateLimitedReader{r: reader, bps: opt.RateLimit}
	}

	n, copyErr := io.Copy(f, reader)
	closeErr := f.Close()
	if copyErr != nil || closeErr != nil {
		// Keep the partial segment for resume
		if copyErr != nil {
			return fmt.Errorf("copying segment body: %w", copyErr)
		}
		return fmt.Errorf("closing segment file: %w", closeErr)
	}
	gd.logf("Segment %s done: %d bytes", segPath, n)
	return nil
}

// countingReader reports the number of bytes read to the callback
type countingReader struct {
	r      io.Reader
	onRead func(int64)
}

func (c *countingReader) Read(b []byte) (int, error) {
	n, err := c.r.Read(b)
	if n > 0 && c.onRead != nil {
		c.onRead(int64(n))
	}
	return n, err
}

// progressCounter returns a thread-safe byte counter that feeds
// the OnProgress callback with the aggregate of all segments
func (gd *GoDownload) progressCounter(opt *Options, total int64) func(int64) {
	if opt == nil || opt.OnProgress == nil {
		return nil
	}
	var mu sync.Mutex
	var sum int64
	return func(n int64) {
		mu.Lock()
		sum += n
		current := sum
		mu.Unlock()
		opt.OnProgress(current, total)
	}
}

// assembleSegments concatenates segment files into outpath atomically
func assembleSegments(segPaths []string, outpath string) error {
	partpath := outpath + partSuffix
	out, err := os.OpenFile(partpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("opening temp file: %w", err)
	}
	for _, sp := range segPaths {
		seg, err := os.Open(sp)
		if err != nil {
			_ = out.Close()
			_ = os.Remove(partpath)
			return fmt.Errorf("opening segment: %w", err)
		}
		_, copyErr := io.Copy(out, seg)
		closeErr := seg.Close()
		if copyErr != nil {
			_ = out.Close()
			_ = os.Remove(partpath)
			return fmt.Errorf("copying segment: %w", copyErr)
		}
		if closeErr != nil {
			_ = out.Close()
			_ = os.Remove(partpath)
			return fmt.Errorf("closing segment: %w", closeErr)
		}
	}
	closeErr := out.Close()
	if closeErr != nil {
		_ = os.Remove(partpath)
		return fmt.Errorf("closing temp file: %w", closeErr)
	}
	// Remove segment files
	for _, sp := range segPaths {
		_ = os.Remove(sp)
	}
	if err := os.Rename(partpath, outpath); err != nil {
		_ = os.Remove(partpath)
		return fmt.Errorf("renaming temp file: %w", err)
	}
	return nil
}

// postProcess applies checksum verification (when not already done)
// and archiving. It returns the final path of the artifact (the
// archive path when archiving is applied) and whether the checksum
// verification succeeded
func (gd *GoDownload) postProcess(opt *Options, outpath string, alreadyVerified bool) (string, bool, error) {
	verified := alreadyVerified
	if opt != nil && opt.Checksum != "" && !alreadyVerified {
		if err := verifyChecksum(outpath, opt.Checksum); err != nil {
			_ = os.Remove(outpath)
			return "", false, err
		}
		verified = true
	}
	if opt != nil && opt.Archive != "" {
		if err := archivePack(outpath, opt.Archive); err != nil {
			return "", verified, fmt.Errorf("packing file: %w", err)
		}
		if err := os.Remove(outpath); err != nil {
			return "", verified, fmt.Errorf("removing original file after packing: %w", err)
		}
		return outpath + "." + opt.Archive, verified, nil
	}
	return outpath, verified, nil
}

// fileSize returns the size of the file at path, 0 on any failure
func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
