package godownload

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
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
	if err := ctx.Err(); err != nil {
		return err
	}
	if gd.Configpath != "" {
		opta, err := loadConfig(gd.Configpath)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
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
		}
	}

	if opt.Page {
		result, err := gd.pageDownload(ctx, path)
		if err != nil {
			return err
		}

		outpath := opt.Outpath
		if outpath == "" {
			outpath, err = getFileNameFromURL(path)
			if err != nil {
				return err
			}
		}
		if err := os.WriteFile(outpath, []byte(result), 0o644); err != nil {
			return fmt.Errorf("writing page to file: %w", err)
		}
		gd.logf("Finish to download from %s", path)
		return nil
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
	errs := make([]error, len(items))
	var wg sync.WaitGroup
	for i, item := range items {
		wg.Add(1)
		go func(idx int, it *Options) {
			defer wg.Done()
			errs[idx] = gd.DownloadContext(ctx, it.URL, it)
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
		return errors.Join(multi...)
	}
	return nil
}

// DownloadManySimple is identical for DownloadMany, but as arguments is slice of url
func (gd *GoDownload) DownloadManySimple(items []string) error {
	return gd.DownloadManySimpleContext(context.Background(), items)
}

// DownloadManySimpleContext is the context-aware version
// of DownloadManySimple
func (gd *GoDownload) DownloadManySimpleContext(ctx context.Context, items []string) error {
	result := make([]*Options, 0, len(items))
	for _, item := range items {
		name, err := getFileNameFromURL(item)
		if err != nil {
			return err
		}
		result = append(result, &Options{URL: item, Outpath: name})
	}
	return gd.DownloadManyContext(ctx, result)
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

// fileDownload method provides downloading files.
// When opt.mirrors is set, the primary URL is tried first and each
// mirror is tried in order until one succeeds
func (gd *GoDownload) fileDownload(ctx context.Context, path string, opt *Options) error {
	err := gd.fileDownloadSingle(ctx, path, opt)
	if err == nil {
		return nil
	}
	for _, mirror := range opt.mirrors {
		gd.logf("Primary URL failed (%v), trying mirror %s", err, mirror)
		err = gd.fileDownloadSingle(ctx, mirror, opt)
		if err == nil {
			return nil
		}
	}
	return err
}

// fileDownloadSingle performs a download from a single URL
func (gd *GoDownload) fileDownloadSingle(ctx context.Context, path string, opt *Options) error {
	if gd.Outdir != "" {
		if err := createDir(gd.Outdir); err != nil {
			return fmt.Errorf("creating outdir: %w", err)
		}
	}

	outpath, err := outpathResolver(path, opt)
	if err != nil {
		return err
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

	// Multi-segment download: split the file into ranged parallel
	// segments when the server supports it
	if opt != nil && opt.Segments > 1 {
		return gd.multiSegmentDownload(ctx, path, opt, outpath, starttime)
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

	resp, err := downloadGeneral(ctx, gd.client(), retry, path, useragent, auth, rangeset, nil)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			gd.logf("closing response body: %v", cerr)
		}
	}()

	if err := checkStatusCode(resp, path); err != nil {
		return err
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
		return err
	}
	gd.logf("Finish to download from %s in %s. Transferred bytes: %d", path,
		time.Since(starttime), transferred)

	if opt != nil && opt.Archive != "" {
		if err := archivePack(outpath, opt.Archive); err != nil {
			return fmt.Errorf("packing file: %w", err)
		}
		if err := os.Remove(outpath); err != nil {
			return fmt.Errorf("removing original file after packing: %w", err)
		}
	}
	return nil
}

// pageDownload downloads the page contents
func (gd *GoDownload) pageDownload(ctx context.Context, path string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	response, err := gd.client().Do(req)
	if err != nil {
		return "", err
	}

	defer func() {
		if cerr := response.Body.Close(); cerr != nil {
			gd.logf("closing response body: %v", cerr)
		}
	}()
	if err := checkStatusCode(response, path); err != nil {
		return "", err
	}

	contents, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}

	return string(contents), nil
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
