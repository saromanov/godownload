package godownload

import (
	"archive/zip"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"errors"
	"fmt"
	"hash"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// Version — версия библиотеки.
const Version = "2.0.0"

// defaultUserAgent используется, если пользователь не задал свой User-Agent.
const defaultUserAgent = "godownload/" + Version

// defaultRetryPause — начальная пауза между попытками скачивания (с экспоненциальным ростом).
const defaultRetryPause = 100 * time.Millisecond

// partSuffix is appended to the target file name while the download
// is in progress (atomic writes)
const partSuffix = ".part"

// progressUpdateInterval defines how often the progress callback fires
const progressUpdateInterval = 64 * 1024

// progressReader wraps a reader and calls the progress callback
// every progressUpdateInterval bytes and once at EOF
type progressReader struct {
	r           io.Reader
	onProgress  func(transferred, total int64)
	total       int64
	transferred int64
	sinceNotify int64
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 {
		p.transferred += int64(n)
		p.sinceNotify += int64(n)
		if p.sinceNotify >= progressUpdateInterval || err != nil {
			p.onProgress(p.transferred, max64(p.total, 0))
			p.sinceNotify = 0
		}
	}
	return n, err
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// rateLimitedReader caps the read speed at bps bytes per second
// with a token-bucket that refills continuously
type rateLimitedReader struct {
	r      io.Reader
	bps    int64
	tokens float64
	last   time.Time
	init   bool
}

func (r *rateLimitedReader) Read(b []byte) (int, error) {
	if !r.init {
		r.last = time.Now()
		r.tokens = float64(r.bps) / 10 // small initial burst
		r.init = true
	}
	if len(b) == 0 {
		return 0, nil
	}
	// Cap the chunk size for smooth pacing
	maxchunk := r.bps / 10
	if maxchunk < 1 {
		maxchunk = 1
	}
	want := int64(len(b))
	if want > maxchunk {
		want = maxchunk
	}

	for {
		now := time.Now()
		r.tokens += now.Sub(r.last).Seconds() * float64(r.bps)
		r.last = now
		if r.tokens > float64(r.bps) {
			r.tokens = float64(r.bps)
		}
		if r.tokens >= float64(want) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	r.tokens -= float64(want)

	return r.r.Read(b[:want])
}

// ErrEmptyOutpath — ошибка, когда для скачивания страницы не задан путь вывода.
var ErrEmptyOutpath = errors.New("outpath is not specified")

// ErrFileExists — ошибка, когда целевой файл уже существует, а перезапись запрещена.
var ErrFileExists = errors.New("file already exists")

// ErrUnsupportedArchive — ошибка неподдерживаемого формата архива.
var ErrUnsupportedArchive = errors.New("unsupported archive format")

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
}

// Download provides downloading of file from path.
// Instead of terminating the process on error, it returns an error
func (gd *GoDownload) Download(path string, opt *Options) error {
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
		result, err := gd.pageDownload(path)
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

	return gd.fileDownload(path, opt)
}

// DownloadMany provides downloading several files concurrently.
// Errors of individual downloads are collected and returned
// as a single aggregated error
func (gd *GoDownload) DownloadMany(items []*Options) error {
	errs := make([]error, len(items))
	var wg sync.WaitGroup
	for i, item := range items {
		wg.Add(1)
		go func(idx int, it *Options) {
			defer wg.Done()
			errs[idx] = gd.Download(it.URL, it)
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
	result := make([]*Options, 0, len(items))
	for _, item := range items {
		name, err := getFileNameFromURL(item)
		if err != nil {
			return err
		}
		result = append(result, &Options{URL: item, Outpath: name})
	}
	return gd.DownloadMany(result)
}

// FromFile provides getting links from file and downloading
func (gd *GoDownload) FromFile(path string) error {
	urls, err := fromFile(path)
	if err != nil {
		return err
	}
	return gd.DownloadManySimple(urls)
}

// fileDownload method provides downloading files
func (gd *GoDownload) fileDownload(path string, opt *Options) error {
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
		return gd.multiSegmentDownload(path, opt, outpath, starttime)
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

	resp, err := downloadGeneral(gd.client(), retry, path, useragent, auth, rangeset, nil)
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

// downloadToFile writes the response body to outpath atomically:
// data goes to a temporary ".part" file which is renamed to the
// final name only after a successful write. On any failure the
// target file is left untouched and the temp file is removed.
// If offset is not nil, the body continues an interrupted download
// and the temp file is appended to instead of being truncated.
// If checksum is not empty ("<algorithm>:<hex>"), the fully written
// temp file is verified before the rename; on mismatch the temp file
// is removed and ErrChecksumMismatch is returned
func (gd *GoDownload) downloadToFile(resp io.Reader, outpath string, offset *int64, checksum string) (int64, error) {
	partpath := outpath + partSuffix
	flags := os.O_WRONLY | os.O_CREATE
	if offset != nil {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}
	f, err := os.OpenFile(partpath, flags, 0o644)
	if err != nil {
		return 0, fmt.Errorf("opening temp file: %w", err)
	}

	n, copyErr := io.Copy(f, resp)
	closeErr := f.Close()
	if copyErr != nil {
		// Keep the temp file: a subsequent download can resume from it
		return n, fmt.Errorf("copying response body: %w", copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(partpath)
		return n, fmt.Errorf("closing temp file: %w", closeErr)
	}

	if checksum != "" {
		if err := verifyChecksum(partpath, checksum); err != nil {
			// A corrupted file must not survive as .part: remove it
			_ = os.Remove(partpath)
			return n, err
		}
	}

	if err := os.Rename(partpath, outpath); err != nil {
		_ = os.Remove(partpath)
		return n, fmt.Errorf("renaming temp file: %w", err)
	}
	return n, nil
}

// ErrChecksumMismatch is returned when the downloaded file does not
// match the expected checksum
var ErrChecksumMismatch = errors.New("checksum mismatch")

// verifyChecksum checks the file hash against "<algorithm>:<hex>"
func verifyChecksum(path, expected string) error {
	algohex, ok := strings.CutPrefix(expected, "sha256:")
	algo := "sha256"
	if !ok {
		algohex, ok = strings.CutPrefix(expected, "sha512:")
		algo = "sha512"
	}
	if !ok {
		algohex, ok = strings.CutPrefix(expected, "sha1:")
		algo = "sha1"
	}
	if !ok {
		algohex, ok = strings.CutPrefix(expected, "md5:")
		algo = "md5"
	}
	if !ok {
		return fmt.Errorf("unsupported checksum format %q, use \"<algorithm>:<hex>\"", expected)
	}

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("opening file for checksum: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			log.Printf("closing file after checksum: %v", cerr)
		}
	}()

	h := cryptoHash(algo)
	if h == nil {
		return fmt.Errorf("unsupported checksum algorithm %q", algo)
	}
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("hashing file: %w", err)
	}
	got := fmt.Sprintf("%x", h.Sum(nil))
	if !strings.EqualFold(got, algohex) {
		return fmt.Errorf("%w: got %s, want %s", ErrChecksumMismatch, got, algohex)
	}
	return nil
}

func cryptoHash(algo string) hash.Hash {
	switch strings.ToLower(algo) {
	case "md5":
		return md5.New()
	case "sha1":
		return sha1.New()
	case "sha256":
		return sha256.New()
	case "sha512":
		return sha512.New()
	}
	return nil
}

// ErrNoRangeSupport is returned when Segments > 1 but the server
// does not support Range requests
var ErrNoRangeSupport = errors.New("server does not support range requests")

// minSegmentSize is the smallest split allowed per segment
const minSegmentSize = 1024 * 1024

// multiSegmentDownload splits the file into opt.Segments ranged
// parts, downloads them in parallel into separate temp files and
// assembles the result into outpath. It falls back to a plain
// single-stream download when the server does not advertise
// support for Range requests
func (gd *GoDownload) multiSegmentDownload(path string, opt *Options, outpath string, starttime time.Time) error {
	retry := opt.Retry
	useragent := defaultUserAgent
	if opt.UserAgent != "" {
		useragent = opt.UserAgent
	}

	// Probe: request the first byte to learn the size and Range support
	probe, err := downloadGeneral(gd.client(), retry, path, useragent, opt.Auth, nil, nil)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := probe.Body.Close(); cerr != nil {
			gd.logf("closing probe body: %v", cerr)
		}
	}()
	if err := checkStatusCode(probe, path); err != nil {
		return err
	}

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
			return err
		}
		gd.logf("Finish to download from %s in %s. Transferred bytes: %d", path,
			time.Since(starttime), transferred)
		return gd.postProcess(opt, outpath)
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
			if err := gd.downloadSegment(path, opt, retry, useragent, start, end, segPath, sharedProgress); err != nil {
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
		return errors.Join(segErrs...)
	}

	// Assemble segments into the final file atomically
	if err := assembleSegments(segPaths, outpath); err != nil {
		return err
	}
	gd.logf("Finish to download from %s in %s. Transferred bytes: %d (%d segments)",
		path, time.Since(starttime), total, segments)
	return gd.postProcess(opt, outpath)
}

// downloadSegment fetches one ranged part of the file into segPath
func (gd *GoDownload) downloadSegment(path string, opt *Options, retry int, useragent string, start, end int64, segPath string, progress func(int64)) error {
	startpos := start
	// Resume the segment if a partial file exists
	if info, err := os.Stat(segPath); err == nil {
		startpos = start + info.Size()
		gd.logf("Resuming segment %s from byte %d", segPath, startpos)
	}
	if startpos > end {
		return nil // segment already complete
	}

	resp, err := downloadGeneral(gd.client(), retry, path, useragent, opt.Auth, &startpos, &end)
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

// postProcess applies checksum verification and archiving
// after the file has landed under its final name
func (gd *GoDownload) postProcess(opt *Options, outpath string) error {
	if opt != nil && opt.Checksum != "" {
		if err := verifyChecksum(outpath, opt.Checksum); err != nil {
			_ = os.Remove(outpath)
			return err
		}
	}
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
func (gd *GoDownload) pageDownload(path string) (string, error) {
	response, err := gd.client().Get(path)
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

// client returns http client for this instance
func (gd *GoDownload) client() *http.Client {
	if gd.HTTPClient != nil {
		return gd.HTTPClient
	}
	return &http.Client{Timeout: gd.Timeout}
}

// logf writes a log message, unless Quiet is set
func (gd *GoDownload) logf(format string, args ...any) {
	if gd.Quiet {
		return
	}
	logger := gd.Logger
	if logger == nil {
		logger = log.Default()
	}
	logger.Printf(format, args...)
}

// drainBody drains and closes the response body
func drainBody(body io.ReadCloser) {
	_, _ = io.Copy(io.Discard, body)
	_ = body.Close()
}

// checkStatusCode validates the HTTP status code of the response
func checkStatusCode(resp *http.Response, path string) error {
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		drainBody(resp.Body)
		return fmt.Errorf("unexpected status code %d from %s", resp.StatusCode, path)
	}
	return nil
}

// Main inner method for downloading with retries and exponential backoff.
// Retries happen on transport errors and on 5xx responses.
// If rangeset is not nil, a Range header is sent and a 206 response
// is expected from servers that support resumable downloads
func downloadGeneral(client *http.Client, retry int, url, useragent, auth string, rangeset *int64, rangeEnd *int64) (*http.Response, error) {
	retrynums := 0
	pause := defaultRetryPause
	for {
		res, err := download(client, url, useragent, auth, rangeset, rangeEnd)
		if err == nil {
			if res.StatusCode >= 500 && res.StatusCode < 600 {
				drainBody(res.Body)
				if retry == 0 || retrynums == retry {
					return nil, fmt.Errorf("unexpected status code %d from %s", res.StatusCode, url)
				}
				log.Printf("Tried again to download from %s: status %d", url, res.StatusCode)
				time.Sleep(pause)
				pause *= 2
				retrynums++
				continue
			}
			return res, nil
		}
		if retry == 0 || retrynums == retry {
			return nil, err
		}
		log.Printf("Tried again to download from %s: %v", url, err)
		time.Sleep(pause)
		pause *= 2
		retrynums++
	}
}

// download performs a single GET request
func download(client *http.Client, url, useragent, auth string, rangeset *int64, rangeEnd *int64) (*http.Response, error) {
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	if rangeset != nil {
		if rangeEnd != nil {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", *rangeset, *rangeEnd))
		} else {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", *rangeset))
		}
	}

	if auth != "" {
		res := strings.SplitN(auth, ":", 2)
		if len(res) != 2 {
			return nil, errors.New("authentication must be in the format username:password")
		}
		req.SetBasicAuth(res[0], res[1])
	}

	if useragent != "" {
		req.Header.Set("User-Agent", useragent)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func getFileNameFromURL(urlitem string) (string, error) {
	res, err := url.Parse(urlitem)
	if err != nil {
		return "", fmt.Errorf("parsing url: %w", err)
	}
	if res.Path == "" || strings.HasSuffix(res.Path, "/") {
		return "", fmt.Errorf("unable to resolve filename from url %s", urlitem)
	}
	items := strings.Split(res.Path, "/")
	name := items[len(items)-1]
	if name == "" {
		return "", fmt.Errorf("unable to resolve filename from url %s", urlitem)
	}
	return name, nil
}

// outpathResolver provides correct outpath for downloaded file
// It's done for better view of the Download method
func outpathResolver(path string, item *Options) (string, error) {
	if item == nil {
		outpath, err := getFileNameFromURL(path)
		if err != nil {
			return "", err
		}
		if checkExist(outpath) {
			return "", fmt.Errorf("%w: %s, set Options.Overwrite = true to overwrite", ErrFileExists, outpath)
		}
		return outpath, nil
	}

	//Default value for outpath: if not set, resolve from URL
	outpath := item.Outpath
	if outpath == "" {
		name, err := getFileNameFromURL(path)
		if err != nil {
			return "", err
		}
		outpath = name
	}

	//Check if outpath exists
	if checkExist(outpath) {
		switch {
		case item.Alwaysnew:
			outpath = nextFreeName(outpath)
		case item.Overwrite:
			// Перезаписываем существующий файл
		default:
			return "", fmt.Errorf("%w: %s, set Options.Overwrite = true to overwrite", ErrFileExists, outpath)
		}
	}

	return outpath, nil
}

// nextFreeName returns the name of the file that is not busy,
// adding a numeric suffix like name_1, name_2, ...
func nextFreeName(outpath string) string {
	ext := filepath.Ext(outpath)
	base := outpath
	if ext != "" {
		base = outpath[:len(outpath)-len(ext)]
	}
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s_%d%s", base, i, ext)
		if !checkExist(candidate) {
			return candidate
		}
	}
}

// archivePack packs the output file into an archive of the given format.
// Supported formats: zip
func archivePack(path, format string) error {
	switch strings.ToLower(format) {
	case "zip":
		return zipPack(path)
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedArchive, format)
	}
}

// Pack output files to zip archive
func zipPack(path string) error {
	newfile, err := os.Create(path + ".zip")
	if err != nil {
		return err
	}
	defer func() {
		if cerr := newfile.Close(); cerr != nil {
			log.Printf("closing archive file: %v", cerr)
		}
	}()
	zipit := zip.NewWriter(newfile)
	defer func() {
		if cerr := zipit.Close(); cerr != nil {
			log.Printf("closing zip writer: %v", cerr)
		}
	}()
	zipfile, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := zipfile.Close(); cerr != nil {
			log.Printf("closing source file: %v", cerr)
		}
	}()
	info, err := zipfile.Stat()
	if err != nil {
		return err
	}
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Method = zip.Deflate

	writer, err := zipit.CreateHeader(header)
	if err != nil {
		return err
	}
	if _, err := io.Copy(writer, zipfile); err != nil {
		return fmt.Errorf("copying data to archive: %w", err)
	}
	if err := zipit.Close(); err != nil {
		return fmt.Errorf("closing archive: %w", err)
	}
	log.Printf("Output as %s", path+".zip")
	return nil
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

// createDir creates a directory for downloading
func createDir(dirname string) error {
	if _, err := os.Stat(dirname); os.IsNotExist(err) {
		if err := os.MkdirAll(dirname, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func checkExist(path string) bool {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false
	}
	return true
}
