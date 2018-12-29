package godownload

import (
	"archive/zip"
	"bytes"
	"errors"
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

	"gopkg.in/yaml.v2"
)

//GoDownload provides main struct for global config and downloading items
type GoDownload struct {

	//Overwrite provides overwriting file with same name
	Overwrite bool

	//Always create new file. If file with same name exist
	// create "file_1"
	Alwaysnew bool

	//UserAgent provides setting user agent for http request
	UserAgent string

	//Retry provides number of attempts to download file
	Retry int

	//Authentication before downloading. Auth in the format username:password
	Auth string

	//Specify archive format for downloaded file
	Archive string

	//Path to the config file
	Configpath string

	//Directory for downloaded file
	Outdir string
}

type Options struct {

	// URL parameter needs only for DownloadMany.
	// In the case with Download. This paremeter will be ignore
	URL string

	//Download web page instead file
	Page bool

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

	//Authentication before downloading. Auth in the format username:password
	Auth string

	//Specify archive format for downloaded file
	Archive string

	//TODO
	TimeLimit time.Time
}

//Downloading provides file downloading
func (gd *GoDownload) Download(path string, opt *Options) {
	if gd.Configpath != "" {
		opta, err := loadConfig(gd.Configpath)
		if err != nil {
			log.Fatal(err)
		}
		gd = opta
	}

	if opt == nil {
		opt = &Options{
			Overwrite: gd.Overwrite,
			Alwaysnew: gd.Alwaysnew,
			UserAgent: gd.UserAgent,
			Retry:     gd.Retry,
			Auth:      gd.Auth,
			Archive:   gd.Archive,
		}
	}

	if opt.Page {
		result, err := gd.pageDownload(path, opt)
		if err != nil {
			log.Fatal(err)
		}

		createTargetFile(opt.Outpath)

		starttime := time.Now()
		errwrite := ioutil.WriteFile(opt.Outpath, []byte(result), 0777)
		if errwrite != nil {
			log.Fatal(errwrite)
		}
		log.Printf("Finish to download from %s in %s.", path,
			time.Since(starttime))

	} else {
		gd.fileDownload(path, opt)
	}
}

//DownloadMany provides downloading several files
func (gd *GoDownload) DownloadMany(items []*Options) {
	runtime.GOMAXPROCS(runtime.NumCPU())
	var wg sync.WaitGroup
	for _, item := range items {
		wg.Add(1)
		go func(it *Options) {
			gd.Download(it.URL, it)
			wg.Done()
		}(item)
	}
	wg.Wait()
}

//DownloadManySimple is identical for DownloadMany, but as arguments is slice of url
func (gd *GoDownload) DownloadManySimple(items []string) {
	result := []*Options{}
	for _, item := range items {
		result = append(result, &Options{URL: item, Outpath: getFileNameFromURL(item)})
	}
	gd.DownloadMany(result)
}

//FromFile provides getting links from file and download
func (gd *GoDownload) FromFile(path string) {
	urls := fromFile(path)
	gd.DownloadManySimple(urls)
}

//fileDownload method provides downloading files
func (gd *GoDownload) fileDownload(path string, opt *Options) {
	if gd.Outdir != "" {
		createDir(gd.Outdir)
		if opt.Outpath != "" {
			opt.Outpath = fmt.Sprintf("%s/%s", gd.Outdir, opt.Outpath)
		} else {
			opt.Outpath = gd.Outdir
		}
	}

	outpath := outpathResolver(path, opt)
	fmt.Println(outpath)

	//Last chance to check if outpath is not empty
	if outpath == "" {
		log.Fatal("Something wen wrong and outpath is empty")
	}

	createTargetFile(outpath)
	retry := 0
	useragent := ""
	auth := ""
	if opt != nil {
		retry = opt.Retry
		useragent = opt.UserAgent
		auth = opt.Auth
	}
	log.Printf("Start to download from %s", path)
	starttime := time.Now()
	resp, err := downloadGeneral(retry, path, useragent, auth)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	transfered := copyToFile(resp.Body, outpath)
	log.Printf("Finish to download from %s in %s. Transfered bytes: %d", path,
		time.Since(starttime), transfered)
	if opt != nil && opt.Archive == "zip" {
		err := zipPack(outpath)
		if err != nil {
			log.Printf("Error to create zeip archive")
			return
		}
		os.Remove(outpath)
	}
}

func (gd *GoDownload) pageDownload(path string, opt *Options) (string, error) {
	if opt.Outpath != "" {
		return "", errors.New("outpath not specified")
	}

	response, err := http.Get(path)
	if err != nil {
		return "", err
	}

	defer response.Body.Close()
	contents, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return "", err
	}

	return string(contents), nil
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

//Main inner method for downloading
func download(url, useragent, auth string) (*http.Response, error) {
	client := http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	if auth != "" {
		res := strings.Split(auth, ":")
		if len(res) != 2 {
			return nil, errors.New("authentication must be in the format username:password")
		}
		req.SetBasicAuth(res[0], res[1])
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

//Set timer for checking
func timer(num int) {
	timer1 := time.NewTimer(time.Duration(num) * time.Second)
	expired := make(chan bool)
	go func() {
		<-timer1.C
		expired <- true
	}()
}

func downloadGeneral(retry int, url, useragent, auth string) (*http.Response, error) {
	retrynums := 0
	for {
		res, err := download(url, useragent, auth)
		if err == nil {
			return res, nil
		} else if retry == 0 {
			return nil, err
		} else {
			if retrynums == retry {
				return nil, err
			}
		}
		fmt.Printf("Tried again to download from %s\n", url)
		retrynums++
		time.Sleep(100 * time.Millisecond)
	}
}

//copy to file
func copyToFile(resp io.Reader, outpath string) int {
	dst := &bytes.Buffer{}

	_, err := io.Copy(dst, resp)
	if err != nil {
		panic(err)
	}

	errwrite := ioutil.WriteFile(outpath, dst.Bytes(), 0777)
	if errwrite != nil {
		log.Fatal(errwrite)
	}
	return dst.Len()
}

func getFileNameFromURL(urlitem string) string {
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
	if item == nil {
		outpath = getFileNameFromURL(path)
		if checkExist(outpath) {
			log.Fatalf("File %s already exist. You can set Options.Overwrite = true for overwrite this file", path)
		}
		return

	}
	//Default value for outpath
	outpath = item.Outpath

	//Check if outpath is exist
	if checkExist(item.Outpath) {
		if item.Alwaysnew {
			ext := filepath.Ext(outpath)
			if ext == "" {
				name := getFileNameFromURL(path)
				outpath = outpath + "/" + name
			}
			dupcount := fileCount(outpath)
			newname := outpath[0:len(outpath)-len(ext)] +
				fmt.Sprintf("_%d", dupcount+1)
			if len(ext) > 0 {
				newname = newname + ext
			}
			if filepath.Dir(outpath) == "." {
				outpath = filepath.Dir(outpath) + "/" + newname
			} else {
				outpath = newname
			}
		} else if !item.Overwrite {
			log.Fatalf("File %s already exist. You can set Options.Overwrite = true for overwrite this file", item.Outpath)
		}
	} else {
		if item.Outpath != "" {
			return outpath
		}

		outpath = getFileNameFromURL(path)
		if item.Overwrite {
			return outpath
		}

		if checkExist(outpath) {
			log.Fatalf("File %s already exist. You can set Options.Overwrite = true for overwrite this file", path)
		}
	}

	return outpath
}

//Pack output files to zip archive
func zipPack(path string) error {
	newfile, err := os.Create(path + ".zip")
	if err != nil {
		return err
	}
	defer newfile.Close()
	zipit := zip.NewWriter(newfile)
	defer zipit.Close()
	zipfile, err := os.Open(path)
	if err != nil {
		return err
	}
	defer zipfile.Close()
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
	_, err = io.Copy(writer, zipfile)
	fmt.Printf("Output as %s\n", path+".zip")
	return err
}

//loadConfig data from .yaml path
func loadConfig(path string) (*GoDownload, error) {
	var opt GoDownload
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	errconf := yaml.Unmarshal(data, &opt)
	if errconf != nil {
		return nil, errconf
	}
	return &opt, nil
}

//create dir for downloading
func createDir(dirname string) {
	if _, err := os.Stat(dirname); os.IsNotExist(err) {
		errmk := os.Mkdir(dirname, 0777)
		if errmk != nil {
			log.Fatal(errmk)
		}
	}
}
