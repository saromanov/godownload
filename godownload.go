package godownload

import (
	"archive/zip"
	"errors"
	"fmt"
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
			Overwrite: gd.Overwrite,
			Alwaysnew: gd.Alwaysnew,
			UserAgent: gd.UserAgent,
			Retry:     gd.Retry,
			Auth:      gd.Auth,
			Archive:   gd.Archive,
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
		// Файл кладём в Outdir, сохраняя имя файла из outpath
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
	resp, err := downloadGeneral(gd.client(), retry, path, useragent, auth)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := checkStatusCode(resp, path); err != nil {
		return err
	}

	transferred, err := copyToFile(resp.Body, outpath)
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
func (gd *GoDownload) pageDownload(path string) (string, error) {
	response, err := gd.client().Get(path)
	if err != nil {
		return "", err
	}

	defer response.Body.Close()
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

// checkStatusCode validates the HTTP status code of the response
func checkStatusCode(resp *http.Response, path string) error {
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		io.Copy(io.Discard, resp.Body)
		return fmt.Errorf("unexpected status code %d from %s", resp.StatusCode, path)
	}
	return nil
}

// Main inner method for downloading with retries and exponential backoff.
// Retries happen on transport errors and on 5xx responses
func downloadGeneral(client *http.Client, retry int, url, useragent, auth string) (*http.Response, error) {
	retrynums := 0
	pause := defaultRetryPause
	for {
		res, err := download(client, url, useragent, auth)
		if err == nil {
			if res.StatusCode >= 500 && res.StatusCode < 600 && (retry == 0 || retrynums == retry) {
				io.Copy(io.Discard, res.Body)
				res.Body.Close()
				return res, nil
			}
			if res.StatusCode >= 500 && res.StatusCode < 600 {
				io.Copy(io.Discard, res.Body)
				res.Body.Close()
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
func download(client *http.Client, url, useragent, auth string) (*http.Response, error) {
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
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

// copyToFile streams the response body to the file
// instead of buffering it fully in memory
func copyToFile(resp io.Reader, outpath string) (int64, error) {
	f, err := os.OpenFile(outpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return 0, fmt.Errorf("opening target file: %w", err)
	}
	defer f.Close()

	n, err := io.Copy(f, resp)
	if err != nil {
		return n, fmt.Errorf("copying response body: %w", err)
	}
	return n, nil
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
	defer newfile.Close()
	zipit := zip.NewWriter(newfile)
	defer zipit.Close()
	zipfile, err := os.Open(path)
	if err != nil {
		return err
	}
	defer zipfile.Close()
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
