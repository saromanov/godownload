package godownload

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Version is the version of the library.
const Version = "2.0.0"

// defaultUserAgent is used when the user has not set their own User-Agent.
const defaultUserAgent = "godownload/" + Version

// defaultRetryPause is the initial pause between download attempts
// (with exponential growth).
const defaultRetryPause = 100 * time.Millisecond

// partSuffix is appended to the target file name while the download
// is in progress (atomic writes)
const partSuffix = ".part"

// GoDownload provides main struct for global config and downloading items
type GoDownload struct {

	//Overwrite provides overwriting file with same name
	Overwrite bool

	//Alwaysnew provides always creating new file.
	// If file with same name exist, create "file_1"
	Alwaysnew bool

	//UserAgent provides setting user agent for http request
	UserAgent string

	//Retry provides number of attempts to download file
	Retry int

	//Authentication before downloading. Auth in the format username:password
	Auth string

	//Specify archive format for downloaded file.
	// Supported values: "zip"
	Archive string

	//OnProgress is called periodically during the download with
	// the number of bytes transferred so far and the total size
	// (0 if the total size is unknown, e.g. chunked responses).
	// Called from the download goroutine; must be cheap or spawn
	// its own goroutine
	OnProgress func(transferred, total int64)

	//Checksum verifies the downloaded file before it becomes visible.
	// Format: "<algorithm>:<hex>", supported algorithms: md5, sha1,
	// sha256, sha512. Example: "sha256:abc123..."
	// An empty value disables verification
	Checksum string

	//RateLimit caps the download speed in bytes per second.
	// Zero means unlimited
	RateLimit int64

	//Segments splits large downloads into this many parallel ranged
	// segments (only when the server supports Range requests).
	// Values < 2 mean a single stream. Default: 1
	Segments int

	//Path to the config file
	Configpath string

	//Directory for downloaded files
	Outdir string

	//Timeout limits the time for the whole request.
	// Zero means no timeout (not recommended)
	Timeout time.Duration

	//Logger is used for all messages of the library.
	// nil means log.Default()
	Logger *log.Logger

	//Quiet disables all logging output
	Quiet bool

	//HTTPClient allows to provide a custom http.Client.
	// If nil, a client with Timeout is used
	HTTPClient *http.Client

	//NoStale skips the download when the server reports the file
	// unchanged since the last download of the same output file
	// (conditional request with If-None-Match / If-Modified-Since
	// based on a sidecar "<outpath>.gd-meta" file). The Result has
	// CacheHit set and Bytes of zero
	NoStale bool

	//OnDone is called once per successfully downloaded file
	// with the outcome of the download (path, size, duration, ...)
	// It is the same Result that the *Result methods return
	OnDone func(Result)
}

// Options defines attributes for config of GoDownload
type Options struct {

	// URL parameter needs only for DownloadMany.
	// In the case with Download. This parameter will be ignored
	URL string

	//Download web page instead file
	Page bool

	//Outpath sets the path of the downloaded file
	Outpath string

	//Overwrite provides overwriting file with same name
	Overwrite bool

	//Alwaysnew provides always creating new file.
	// If file with same name exist, create "file_1"
	Alwaysnew bool

	//UserAgent provides setting user agent for http request
	UserAgent string

	//Retry provides number of attempts to download file
	Retry int

	//Authentication before downloading. Auth in the format username:password
	Auth string

	//Specify archive format for downloaded file.
	// Supported values: "zip"
	Archive string

	//OnProgress is called periodically during the download with
	// the number of bytes transferred so far and the total size
	// (0 if the total size is unknown, e.g. chunked responses).
	// Called from the download goroutine; must be cheap or spawn
	// its own goroutine
	OnProgress func(transferred, total int64)

	//Checksum verifies the downloaded file before it becomes visible.
	// Format: "<algorithm>:<hex>", supported algorithms: md5, sha1,
	// sha256, sha512. Example: "sha256:abc123..."
	// An empty value disables verification
	Checksum string

	//RateLimit caps the download speed in bytes per second.
	// Zero means unlimited
	RateLimit int64

	//Segments splits large downloads into this many parallel ranged
	// segments (only when the server supports Range requests).
	// Values < 2 mean a single stream. Default: 1
	Segments int

	//OnDone is called once per successfully downloaded file
	// with the outcome of the download (path, size, duration, ...)
	// It overrides GoDownload.OnDone for this download
	OnDone func(Result)

	//NoStale skips the download when the server reports the file
	// unchanged since the last download of the same output file.
	// It overrides GoDownload.NoStale for this download
	NoStale bool

	// mirrors lists alternative URLs tried in order when the primary
	// URL fails. Populated by FromManifest
	mirrors []string
}

// Download provides downloading of file from path.
// Instead of terminating the process on error, it returns an error
func (gd *GoDownload) Download(path string, opt *Options) error {
	return gd.DownloadContext(context.Background(), path, opt)
}

// DownloadContext downloads a file from path, honouring ctx cancellation:
// in-flight HTTP requests are aborted and no file is written after
// the context is done
func (gd *GoDownload) DownloadContext(ctx context.Context, path string, opt *Options) error {
	_, err := gd.download(ctx, path, opt)
	return err
}

// DownloadResult is the context-aware download that reports the outcome
func (gd *GoDownload) DownloadResult(ctx context.Context, path string, opt *Options) (Result, error) {
	return gd.download(ctx, path, opt)
}

// download is the internal download entry point: it resolves
// global config, builds effective options and dispatches to the
// page or file downloader
func (gd *GoDownload) download(ctx context.Context, path string, opt *Options) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if gd.Configpath != "" {
		opta, err := loadConfig(gd.Configpath)
		if err != nil {
			return Result{}, fmt.Errorf("loading config: %w", err)
		}
		gd = opta
	}

	if opt == nil {
		opt = &Options{
			Overwrite:  gd.Overwrite,
			Alwaysnew:  gd.Alwaysnew,
			UserAgent:  gd.UserAgent,
			Retry:      gd.Retry,
			Auth:       gd.Auth,
			Archive:    gd.Archive,
			OnProgress: gd.OnProgress,
			Checksum:   gd.Checksum,
			RateLimit:  gd.RateLimit,
			Segments:   gd.Segments,
			OnDone:     gd.OnDone,
			NoStale:    gd.NoStale,
		}
	} else if opt.OnDone == nil {
		opt.OnDone = gd.OnDone
	}
	if !opt.NoStale {
		opt.NoStale = gd.NoStale
	}

	if opt.Page {
		return gd.pageDownload(ctx, path, opt)
	}

	return gd.fileDownload(ctx, path, opt)
}

// DownloadMany provides downloading several files concurrently.
// Errors of individual downloads are collected and returned
// as a single aggregated error
func (gd *GoDownload) DownloadMany(items []*Options) error {
	return gd.DownloadManyContext(context.Background(), items)
}

// DownloadManyContext downloads several files concurrently,
// honouring ctx cancellation
func (gd *GoDownload) DownloadManyContext(ctx context.Context, items []*Options) error {
	_, err := gd.downloadMany(ctx, items)
	return err
}

// DownloadManyResult is the context-aware DownloadMany reporting results.
// On partial failure it returns the results of the successful downloads
// together with the aggregated error
func (gd *GoDownload) DownloadManyResult(ctx context.Context, items []*Options) ([]Result, error) {
	return gd.downloadMany(ctx, items)
}

// DownloadManySimple is identical for DownloadMany, but as arguments is slice of url
func (gd *GoDownload) DownloadManySimple(items []string) error {
	return gd.DownloadManySimpleContext(context.Background(), items)
}

// DownloadManySimpleContext is the context-aware version
// of DownloadManySimple
func (gd *GoDownload) DownloadManySimpleContext(ctx context.Context, items []string) error {
	result, err := manyOptions(items)
	if err != nil {
		return err
	}
	return gd.DownloadManyContext(ctx, result)
}

// DownloadManySimpleResult is the context-aware DownloadManySimple
// reporting results
func (gd *GoDownload) DownloadManySimpleResult(ctx context.Context, items []string) ([]Result, error) {
	opts, err := manyOptions(items)
	if err != nil {
		return nil, err
	}
	return gd.downloadMany(ctx, opts)
}

// FromFile provides getting links from file and downloading
func (gd *GoDownload) FromFile(path string) error {
	return gd.FromFileContext(context.Background(), path)
}

// FromFileContext is the context-aware version of FromFile
func (gd *GoDownload) FromFileContext(ctx context.Context, path string) error {
	urls, err := fromFile(path)
	if err != nil {
		return err
	}
	return gd.DownloadManySimpleContext(ctx, urls)
}

// FromFileResult is the context-aware FromFile reporting results
func (gd *GoDownload) FromFileResult(ctx context.Context, path string) ([]Result, error) {
	urls, err := fromFile(path)
	if err != nil {
		return nil, err
	}
	return gd.DownloadManySimpleResult(ctx, urls)
}

// fileDownload method provides downloading files.
// When opt.mirrors is set, the primary URL is tried first and each
// mirror is tried in order until one succeeds
func (gd *GoDownload) fileDownload(ctx context.Context, path string, opt *Options) (Result, error) {
	res, err := gd.fileDownloadSingle(ctx, path, opt)
	if err == nil {
		return res, nil
	}
	for _, mirror := range opt.mirrors {
		gd.logf("Primary URL failed (%v), trying mirror %s", err, mirror)
		res, err = gd.fileDownloadSingle(ctx, mirror, opt)
		if err == nil {
			return res, nil
		}
	}
	return Result{}, err
}

// fileDownloadSingle performs a download from a single URL
func (gd *GoDownload) fileDownloadSingle(ctx context.Context, path string, opt *Options) (Result, error) {
	if gd.Outdir != "" {
		if err := createDir(gd.Outdir); err != nil {
			return Result{}, fmt.Errorf("creating outdir: %w", err)
		}
	}

	outpath, err := outpathResolver(path, opt)
	if err != nil {
		return Result{}, err
	}
	if gd.Outdir != "" {
		// The file goes to Outdir keeping the base name from outpath
		outpath = filepath.Join(gd.Outdir, filepath.Base(outpath))
		opt.Outpath = outpath
	}

	retry := 0
	useragent := defaultUserAgent
	auth := ""
	if opt != nil {
		retry = opt.Retry
		if opt.UserAgent != "" {
			useragent = opt.UserAgent
		}
		auth = opt.Auth
	}

	gd.logf("Start to download from %s", path)
	starttime := time.Now()

	nostale := opt != nil && opt.NoStale
	cond := cacheHeadersFor(outpath, nostale)

	// Multi-segment download: split the file into ranged parallel
	// segments when the server supports it
	if opt != nil && opt.Segments > 1 {
		return gd.multiSegmentDownload(ctx, path, opt, outpath, starttime, cond)
	}

	// If a temp file from a previous interrupted download exists,
	// try to resume it with a Range request
	partpath := outpath + partSuffix
	var rangeset *int64
	if info, err := os.Stat(partpath); err == nil && !opt.Page {
		offset := info.Size()
		rangeset = &offset
		gd.logf("Resuming download of %s from byte %d", path, offset)
	}

	resp, err := downloadGeneral(ctx, gd.client(), retry, path, useragent, auth, rangeset, nil, cond)
	if err != nil {
		return Result{}, err
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			gd.logf("closing response body: %v", cerr)
		}
	}()

	// The server reports the local file unchanged since the last
	// download: skip the transfer
	if resp.StatusCode == http.StatusNotModified {
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

	if err := checkStatusCode(resp, path); err != nil {
		return Result{}, err
	}

	// Server ignored the Range request and returned the full body:
	// start over from scratch
	if rangeset != nil && resp.StatusCode != http.StatusPartialContent {
		gd.logf("Server %s does not support resume, starting over", path)
		rangeset = nil
	}

	body := io.Reader(resp.Body)
	if opt != nil && opt.RateLimit > 0 {
		body = &rateLimitedReader{r: body, bps: opt.RateLimit}
	}
	if opt != nil && opt.OnProgress != nil {
		// ContentLength is -1 when the size is unknown (chunked responses)
		total := resp.ContentLength
		if total < 0 {
			total = 0
		}
		body = &progressReader{r: body, onProgress: opt.OnProgress, total: total}
	}

	transferred, err := gd.downloadToFile(body, outpath, rangeset, opt.Checksum)
	if err != nil {
		return Result{}, err
	}
	saveCacheMeta(path, outpath, nostale, resp)
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

// pageDownload downloads the page contents into a file and reports Result
func (gd *GoDownload) pageDownload(ctx context.Context, path string, opt *Options) (Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		return Result{}, fmt.Errorf("creating request: %w", err)
	}
	response, err := gd.client().Do(req)
	if err != nil {
		return Result{}, err
	}

	defer func() {
		if cerr := response.Body.Close(); cerr != nil {
			gd.logf("closing response body: %v", cerr)
		}
	}()
	if err := checkStatusCode(response, path); err != nil {
		return Result{}, err
	}

	contents, err := io.ReadAll(response.Body)
	if err != nil {
		return Result{}, err
	}

	outpath := opt.Outpath
	if outpath == "" {
		outpath, err = getFileNameFromURL(path)
		if err != nil {
			return Result{}, err
		}
	}
	start := time.Now()
	if err := os.WriteFile(outpath, contents, 0o644); err != nil {
		return Result{}, fmt.Errorf("writing page to file: %w", err)
	}
	res := Result{
		URL:      path,
		Path:     outpath,
		Size:     int64(len(contents)),
		Bytes:    int64(len(contents)),
		Segments: 1,
		Duration: time.Since(start),
	}
	if opt.OnDone != nil {
		opt.OnDone(res)
	}
	gd.logf("Finish to download from %s", path)
	return res, nil
}

// loadConfig data from .yaml path
func loadConfig(path string) (*GoDownload, error) {
	var opt GoDownload
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(data, &opt); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return &opt, nil
}
