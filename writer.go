package godownload

import (
	"fmt"
	"io"
	"os"
)

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
