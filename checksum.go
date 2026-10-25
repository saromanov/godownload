package godownload

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
	"hash"
	"io"
	"log"
	"os"
	"strings"
)

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
