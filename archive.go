package godownload

import (
	"archive/zip"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

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
