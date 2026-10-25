package godownload

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

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
func downloadGeneral(ctx context.Context, client *http.Client, retry int, url, useragent, auth string, rangeset *int64, rangeEnd *int64) (*http.Response, error) {
	retrynums := 0
	pause := defaultRetryPause
	for {
		res, err := download(ctx, client, url, useragent, auth, rangeset, rangeEnd)
		if err == nil {
			if res.StatusCode >= 500 && res.StatusCode < 600 {
				drainBody(res.Body)
				if retry == 0 || retrynums == retry {
					return nil, fmt.Errorf("unexpected status code %d from %s", res.StatusCode, url)
				}
				log.Printf("Tried again to download from %s: status %d", url, res.StatusCode)
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(pause):
				}
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
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(pause):
		}
		pause *= 2
		retrynums++
	}
}

// download performs a single GET request
func download(ctx context.Context, client *http.Client, url, useragent, auth string, rangeset *int64, rangeEnd *int64) (*http.Response, error) {
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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
