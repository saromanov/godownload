package godownload

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

// metaSuffix is appended to the output path to store freshness
// metadata used by the NoStale option (ETag / Last-Modified)
const metaSuffix = ".gd-meta"

// cacheMeta holds the validator headers of the last successful
// download of the file at outpath
type cacheMeta struct {
	URL          string `json:"url"`
	ETag         string `json:"etag,omitempty"`
	LastModified string `json:"last_modified,omitempty"`
}

// cacheMetaPath returns the sidecar path of the given output file
func cacheMetaPath(outpath string) string {
	return outpath + metaSuffix
}

// readCacheMeta reads the sidecar metadata; nil is returned when
// the file does not exist or cannot be parsed
func readCacheMeta(metaPath string) *cacheMeta {
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return nil
	}
	var meta cacheMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil
	}
	return &meta
}

// writeCacheMeta stores the validator headers atomically:
// the file is written to a temporary name and renamed
func writeCacheMeta(metaPath string, meta *cacheMeta) error {
	if meta == nil || (meta.ETag == "" && meta.LastModified == "") {
		// Nothing to validate against: drop a stale sidecar
		if err := os.Remove(metaPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing stale meta file: %w", err)
		}
		return nil
	}
	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("encoding meta file: %w", err)
	}
	tmp := metaPath + partSuffix
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("writing meta file: %w", err)
	}
	if err := os.Rename(tmp, metaPath); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("renaming meta file: %w", err)
	}
	return nil
}

// condHeaders carries validator headers for a conditional request
type condHeaders struct {
	etag         string
	lastModified string
}

// headers applies the conditional headers to the request.
// Returns false when nothing can be validated
func (c *condHeaders) apply(req *http.Request) bool {
	if c == nil || (c.etag == "" && c.lastModified == "") {
		return false
	}
	if c.etag != "" {
		req.Header.Set("If-None-Match", c.etag)
	}
	if c.lastModified != "" {
		req.Header.Set("If-Modified-Since", c.lastModified)
	}
	return true
}

// cacheHeadersFor returns the conditional headers for outpath when
// the freshness check is enabled and a previous download stored
// validator metadata
func cacheHeadersFor(outpath string, nostale bool) *condHeaders {
	if !nostale {
		return nil
	}
	meta := readCacheMeta(cacheMetaPath(outpath))
	if meta == nil {
		return nil
	}
	return &condHeaders{etag: meta.ETag, lastModified: meta.LastModified}
}

// saveCacheMeta stores the validator headers of the response
// when the freshness check is enabled
func saveCacheMeta(url, outpath string, nostale bool, resp *http.Response) {
	if !nostale {
		return
	}
	etag := resp.Header.Get("ETag")
	lastModified := resp.Header.Get("Last-Modified")
	if etag == "" && lastModified == "" {
		// The server does not support validators: a previously
		// stored sidecar (if any) is no longer valid
		_ = os.Remove(cacheMetaPath(outpath))
		return
	}
	if err := writeCacheMeta(cacheMetaPath(outpath), &cacheMeta{
		URL:          url,
		ETag:         etag,
		LastModified: lastModified,
	}); err != nil {
		// Non-fatal: freshness check is an optimization
		return
	}
}
