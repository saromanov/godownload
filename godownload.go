package godownload

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type Options struct {

	// Url parameter needs only for DownloadMany.
	// In the case with Download. This paremeter will be ignore
	Url string

	//Outpath sets the path of the downloaded file
	Outpath string

	//Overwrite provides overwriting file with same name
	Overwrite bool

	//Always create new file. If file with same name exist
	// create "file_1"
	Alwaysnew bool

	//UserAgent provides setting user agent for http request
	UserAgent string

	//Retry provides number of attempts to download file
	Retry int

	//TODO
	TimeLimit time.Time
}

//Downloading provides file downloading
func Download(path string, item *Options) {
	outpath := outpathResolver(path, item)

	//Last chance to check if outpath is not empty
	if outpath == "" {
		log.Fatal("Something wen wrong and outpath is empty")
	}

	createTargetFile(outpath)
	log.Printf(fmt.Sprintf("Start to download from %s", path))
	starttime := time.Now()
	resp, err := downloadGeneral(item.Retry, path, item.UserAgent)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	transfered := copyToFile(resp, outpath)
	log.Printf(fmt.Sprintf("Finish to download from %s in %s. Transfered bytes: %d", path,
		time.Since(starttime), transfered))
}

//DownloadMany provides downloading several files
func DownloadMany(items []*Options) {
	runtime.GOMAXPROCS(runtime.NumCPU())
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
	fromFile(path)
}

func checkExist(path string) bool {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false
	}

	return true
}

func createTargetFile(path string) {
	res, err := os.Create(path)
	if err != nil {
		panic(err)
	}

	defer res.Close()
}

func download(url string, useragent string) (*http.Response, error) {
	client := http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	if useragent != "" {
		req.Header.Set("User-Agent", useragent)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func timer(num int) {
	timer := time.NewTimer(time.Duration(num) * time.Second)
	expired := make(chan bool)
	go func() {
		<-timer.C
		expired <- true
	}()
}

func downloadGeneral(retry int, url, useragent string) (*http.Response, error) {
	retrynums := 0
	for {
		res, err := download(url, useragent)
		if err == nil {
			return res, nil
		} else if retry == 0 {
			return nil, err
		} else {
			if retrynums == retry {
				return nil, err
			}
		}
		fmt.Println(fmt.Sprintf("Tried again to download from %s", url))
		retrynums += 1
		time.Sleep(100 * time.Millisecond)
	}
}

//copy to file
func copyToFile(resp *http.Response, outpath string) int {
	dst := &bytes.Buffer{}

	_, err := io.Copy(dst, resp.Body)
	if err != nil {
		panic(err)
	}

	errwrite := ioutil.WriteFile(outpath, dst.Bytes(), 0777)
	if errwrite != nil {
		log.Fatal(errwrite)
	}
	return dst.Len()
}

func getFileNameFromUrl(urlitem string) string {
	res, err := url.Parse(urlitem)
	if err != nil {
		panic(err)
	}

	items := strings.Split(res.Path, "/")
	return items[len(items)-1]
}

//outpathResolver provides correct outpath for downloaded file
//It's done for better view of the Download method
func outpathResolver(path string, item *Options) (outpath string) {
	if item != nil {
		//Defeult value for outpath
		outpath = item.Outpath

		//Check if outpath is exist
		if checkExist(item.Outpath) {
			//Also, if we create new file, anyway
			if item.Alwaysnew {
				ext := filepath.Ext(item.Outpath)
				//dupcount always returns non-zero value
				dupcount := fileCount(item.Outpath)
				newname := item.Outpath[0:len(item.Outpath)-len(ext)] +
					fmt.Sprintf("_%d", dupcount+1)
				if len(ext) > 0 {
					newname = newname + ext
				}
				outpath = filepath.Dir(item.Outpath) + "/" + newname
			} else if !item.Overwrite {
				log.Fatal(fmt.Sprintf("File %s already exist. You can set Options.Overwrite = true for overwrite this file", item.Outpath))
			}
		}
	} else {
		outpath = getFileNameFromUrl(path)
		if checkExist(outpath) {
			log.Fatal(fmt.Sprintf("File %s already exist. You can set Options.Overwrite = true for overwrite this file", path))
		}
	}

	return outpath
}
