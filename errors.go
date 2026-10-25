package godownload

import "errors"

// ErrEmptyOutpath — ошибка, когда для скачивания страницы не задан путь вывода.
var ErrEmptyOutpath = errors.New("outpath is not specified")

// ErrFileExists — ошибка, когда целевой файл уже существует, а перезапись запрещена.
var ErrFileExists = errors.New("file already exists")

// ErrUnsupportedArchive — ошибка неподдерживаемого формата архива.
var ErrUnsupportedArchive = errors.New("unsupported archive format")

// ErrChecksumMismatch is returned when the downloaded file does not
// match the expected checksum
var ErrChecksumMismatch = errors.New("checksum mismatch")

// ErrNoRangeSupport is returned when Segments > 1 but the server
// does not support Range requests
var ErrNoRangeSupport = errors.New("server does not support range requests")
