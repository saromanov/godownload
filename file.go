package godownload

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

//Этот модуль реализует получение и разбор ссылок из файла

func prepare(filedata string) string {
	replace := func(check string) string {
		return strings.ReplaceAll(filedata, check, "")
	}

	filedata = replace("(")
	filedata = replace(")")
	filedata = replace(",")
	return filedata
}

// hasURL returns true if item is url and false otherwise
func hasURL(item string) bool {
	if len(item) < 7 {
		return false
	}
	for _, scheme := range []string{"http://", "https://", "ftp://"} {
		if strings.HasPrefix(item, scheme) {
			return true
		}
	}
	return false
}

// validURL checks that the string is a parseable URL with a known scheme
func validURL(item string) bool {
	res, err := url.Parse(item)
	if err != nil || res.Scheme == "" || res.Host == "" {
		return false
	}
	return true
}

// fromFile reads the file and returns a list of unique URLs from it
func fromFile(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	keyurls := make(map[string]bool)
	newdata := prepare(string(data))
	for _, line := range strings.Split(newdata, "\n") {
		for _, part := range strings.Fields(line) {
			_, ok := keyurls[part]
			if hasURL(part) && !ok && validURL(part) {
				keyurls[part] = true
			}
		}
	}

	urls := make([]string, 0, len(keyurls))
	for key := range keyurls {
		urls = append(urls, key)
	}
	return urls, nil
}
