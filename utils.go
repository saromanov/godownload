package godownload

import (
	"os"
	"path/filepath"
)

//Этот модуль содержит некоторые утилиты

// fileCount provides counting duplicates of file.
// File duplicate in this context is file like a file_1.txt, file_2.txt etc
//
// Deprecated: use nextFreeName instead, it resolves names without gaps
func fileCount(path string) int {
	count := 0
	dir := filepath.Dir(path)
	dirdata, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	currentname := filepath.Base(path)
	ext := filepath.Ext(currentname)
	currentname = currentname[:len(currentname)-len(ext)]
	pattern := currentname + "_*"
	if ext != "" {
		pattern = currentname + "_*" + ext
	}

	for _, item := range dirdata {
		ok, _ := filepath.Match(pattern, item.Name())
		if ok {
			count++
		}
	}
	return count + 1
}
