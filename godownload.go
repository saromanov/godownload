package godownload

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
)

type Options struct {
	Url       string
	Outpath   string
	Overwrite bool
}

//Downloading provides file downloading
func Download(path string, item *Options) {
	var outpath string
	if item != nil {
		if checkExist(item.Outpath) && !item.Overwrite {
			log.Fatal(fmt.Sprintf("File %s already exist. You can set Options.Overwrite = true for overwrite this file", item.Outpath))
		}
		outpath = item.Outpath
	} else {
		outpath = getFileNameFromUrl(path)
		if checkExist(outpath) {
			log.Fatal(fmt.Sprintf("File %s already exist. You can set Options.Overwrite = true for overwrite this file", path))
		}
	}

	obj := createTargetFile(outpath)
	defer obj.Close()
	log.Printf(fmt.Sprintf("Start to download from %s", path))
	resp := download(path)
	defer resp.Body.Close()
	copyToFile(resp, obj)
	log.Printf(fmt.Sprintf("Finish to download from %s", path))
}

//DownloadMany provides downloading several files
func DownloadMany(items []*Options) {
	runtime.GOMAXPROCS(2)
	var wg sync.WaitGroup
	for _, item := range items {
		wg.Add(1)
		go func(it *Options) {
			Download(it.Url, it)
			wg.Done()
		}(item)
	}
	wg.Wait()
}

//DownloadManySimple is identical for DownloadMany, but as arguments is slice of url
func DownloadManySimple(items []string) {
	result := []*Options{}
	for _, item := range items {
		result = append(result, &Options{Url: item, Outpath: getFileNameFromUrl(item)})
	}
	DownloadMany(result)
}

//FromFile provides getting links from file and download
func FromFile(path string) {

}

func checkExist(path string) bool {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false
	}

	return true
}

func createTargetFile(path string) *os.File {
	res, err := os.Create(path)
	if err != nil {
		panic(err)
	}

	return res
}

func download(url string) *http.Response {
	resp, err := http.Get(url)
	if err != nil {
		panic(err)
	}
	return resp
}

func copyToFile(resp *http.Response, file *os.File) {
	_, err := io.Copy(file, resp.Body)
	if err != nil {
		panic(err)
	}
}

func getFileNameFromUrl(urlitem string) string {
	res, err := url.Parse(urlitem)
	if err != nil {
		panic(err)
	}

	items := strings.Split(res.Path, "/")
	return items[len(items)-1]
}
