package godownload

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"
)

//http://www.gnu.org/software/wget/manual/wget.html

func exist(path string) bool {
	return checkExist(path)
}

func remove(t *testing.T, path string) {
	err := os.Remove(path)
	if err != nil {
		t.Fatal(err)
	}
}

func createFileWithLinks(filename string) {
	data := `This file contains interesting links!
	First: https://github.com/saromanov/godownload/archive/master.zip \n
	Second: http://arxiv.org/pdf/1206.5538v3.pdf`

	err := ioutil.WriteFile(filename, []byte(data), 0777)
	if err != nil {
		panic(err)
	}
}

func TestDownload(t *testing.T) {
	gd := &GoDownload{}
	gd.Download("https://github.com/saromanov/godownload/archive/master.zip", nil)
	path := "master.zip"
	if !exist(path) {
		t.Fatal(fmt.Sprintf("Downloaded file %s not found", path))
	}
	remove(t, path)
}

func TestDownloadAlwaysNew(t *testing.T) {
	gd := &GoDownload{}
	gd.Download("https://github.com/saromanov/godownload/archive/master.zip", nil)
	gd.Download("https://github.com/saromanov/godownload/archive/master.zip", &Options{
		Outpath: "master.zip", Alwaysnew: true})
	if !exist("master_2.zip") {
		t.Fatal(fmt.Sprintf("Downloaded file %s not found", "master_2.zip"))
	}
	remove(t, "master.zip")
	remove(t, "master_2.zip")
}

func TestDownloadMany(t *testing.T) {
	path1 := "first.zip"
	path2 := "second.zip"
	items := []*Options{
		&Options{Url: "https://github.com/saromanov/godownload/archive/master.zip", Outpath: "first.zip"},
		&Options{Url: "http://arxiv.org/pdf/1206.5538v3.pdf", Outpath: "second.zip"},
	}

	gd := &GoDownload{}
	gd.DownloadMany(items)
	if !exist(path1) {
		t.Fatal(fmt.Sprintf("Downloaded file %s not found", path1))
	}

	if !exist(path2) {
		t.Fatal(fmt.Sprintf("Downloaded file %s not found", path2))
	}

	remove(t, path1)
	remove(t, path2)
}

func TestFromFile(t *testing.T) {
	path1 := "master.zip"
	path2 := "1206.5538v3.pdf"
	createFileWithLinks("simple")

	gd := &GoDownload{}
	gd.FromFile("simple")
	if !exist(path1) {
		t.Fatal(fmt.Sprintf("Downloaded file %s not found", path1))
	}

	if !exist(path2) {
		t.Fatal(fmt.Sprintf("Downloaded file %s not found", path2))
	}

	remove(t, path1)
	remove(t, path2)
	remove(t, "simple")
}
