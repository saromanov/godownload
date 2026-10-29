package godownload

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

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
// It's done for better view of the Download method.
// With NoStale an existing file is allowed: the freshness check
// decides whether it is re-downloaded
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
		case item.Overwrite, item.NoStale:
			// Overwrite the existing file; with NoStale the
			// freshness check may skip the download
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
