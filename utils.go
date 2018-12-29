package godownload

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	//"strings"
)

//This module contains some utils

//fileCount provides counting duplicates of file.
//File duplicate in this context is file like a file_1.txt, file_2.txt etc
//
//TODO: Resolve bug with not names reaching out of order (file_1, file_5)
//After this, new file will create with name file_3, but more correct is file_6
func fileCount(path string) int {
	count := 0
	dir := filepath.Dir(path)
	dirdata, err := ioutil.ReadDir(dir)
	if err != nil {
		panic(err)
	}
	currentname := filepath.Base(path)
	ext := filepath.Ext(currentname)
	currentname = currentname[0 : len(currentname)-len(ext)]
	pattern := fmt.Sprintf("%s_*", currentname)
	if ext != "" {
		pattern = fmt.Sprintf("%s_*%s", currentname, ext)
	}

	for _, item := range dirdata {
		ok, _ := filepath.Match(pattern, item.Name())
		if ok {
			count++
		}
	}
	return count + 1
}
