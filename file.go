package godownload

import (
	"fmt"
	"io/ioutil"
	"log"
	"strings"
)

//This module implemented getting and parsing urls from file

func prepare(filedata string) string {
	replace := func(check string) string {
		return strings.Replace(filedata, check, "", -1)
	}

	filedata = replace("(")
	filedata = replace(")")
	filedata = replace(",")
	return filedata
}

//hashURL returns true if item is url and false otherwise
func hasURL(item string) bool {
	prefix := func(check string) bool {
		return strings.HasPrefix(item, check)
	}
	return prefix("http") || prefix("https") || prefix("ftp")
}

//FromFile provides getting links from file and download
func fromFile(path string) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		panic(err)
	}

	keyurls := map[string]bool{}
	fmt.Println(string(data))
	newdata := prepare(string(data))
	for _, line := range strings.Split(newdata, "\n") {
		for _, part := range strings.Split(line, " ") {
			_, ok := keyurls[part]
			if hasURL(part) && !ok {
				keyurls[part] = true
			}
		}
	}
	log.Printf(fmt.Sprintf("In the file %s, found URLs: %d", path, len(keyurls)))
	fmt.Println("URLs: ")
	urls := []string{}
	for key := range keyurls {
		fmt.Println(key)
		urls = append(urls, key)
	}
	fmt.Println("\n")

	DownloadManySimple(urls)
}
