package godownload

import (
	"context"
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ManifestItem describes a single download entry in a YAML manifest
type ManifestItem struct {
	// URL of the file to download (required)
	URL string `yaml:"url"`
	// Out is the output path of the downloaded file (required)
	Out string `yaml:"out"`
	// Checksum in the "<algorithm>:<hex>" format (optional)
	Checksum string `yaml:"checksum,omitempty"`
	// Mirrors is a list of alternative URLs (optional).
	// The first URL that responds is used; the primary URL
	// comes first implicitly
	Mirrors []string `yaml:"mirrors,omitempty"`
	// Retry overrides the global retry count (optional)
	Retry int `yaml:"retry,omitempty"`
	// Segments overrides the global segment count (optional)
	Segments int `yaml:"segments,omitempty"`
	// RateLimit overrides the global rate limit (optional)
	RateLimit int64 `yaml:"rate_limit,omitempty"`
	// Archive overrides the global archive format (optional)
	Archive string `yaml:"archive,omitempty"`
	// Auth overrides the global auth (optional)
	Auth string `yaml:"auth,omitempty"`
}

// FromManifest downloads all files declared in a YAML manifest:
//
//   - url: https://example.com/model.bin
//     out: model.bin
//     checksum: sha256:abc...
//     mirrors:
//   - https://mirror.example.com/model.bin
//
// Items are downloaded concurrently; per-item settings override
// the global GoDownload config
func (gd *GoDownload) FromManifest(manifestpath string) error {
	return gd.FromManifestContext(context.Background(), manifestpath)
}

// FromManifestContext is the context-aware version of FromManifest
func (gd *GoDownload) FromManifestContext(ctx context.Context, manifestpath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := os.ReadFile(manifestpath)
	if err != nil {
		return fmt.Errorf("reading manifest: %w", err)
	}
	var items []ManifestItem
	if err := yaml.Unmarshal(data, &items); err != nil {
		return fmt.Errorf("parsing manifest: %w", err)
	}
	if len(items) == 0 {
		return errors.New("manifest is empty")
	}

	opts := make([]*Options, 0, len(items))
	for i, item := range items {
		if item.URL == "" {
			return fmt.Errorf("manifest item %d: url is required", i)
		}
		if item.Out == "" {
			name, err := getFileNameFromURL(item.URL)
			if err != nil {
				return fmt.Errorf("manifest item %d: out is required", i)
			}
			item.Out = name
		}
		opts = append(opts, &Options{
			URL:       item.URL,
			Outpath:   item.Out,
			Checksum:  item.Checksum,
			Retry:     item.Retry,
			Segments:  item.Segments,
			RateLimit: item.RateLimit,
			Archive:   item.Archive,
			Auth:      item.Auth,
		})
		if len(item.Mirrors) > 0 {
			opts[len(opts)-1].mirrors = item.Mirrors
		}
	}
	return gd.DownloadManyContext(ctx, opts)
}
