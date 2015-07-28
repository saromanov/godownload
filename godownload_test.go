package godownload

import
(
	"testing"
	"os"
	"fmt"
)

func exist(path string) bool {
	return checkExist(path)
}

func remove(t *testing.T, path string){
	err := os.Remove(path)
	if err != nil {
		t.Fatal(err)
	}
}


func TestDownload(t *testing.T){
	Download("https://github.com/saromanov/godownload/archive/master.zip", nil)
	path := "master.zip"
	if !exist(path) {
		t.Fatal(fmt.Sprintf("Downloaded file %s not found", path))
	}
	remove(t, path)
}

func TestDownloadMany (t *testing. T) {
	path1 := "first.zip"
	path2 := "second.zip"
	items := []*Options{ 
		&Options{Url: "https://github.com/saromanov/godownload/archive/master.zip", Outpath: "first.zip"}, 
		&Options{Url: "http://arxiv.org/pdf/1206.5538v3.pdf", Outpath: "second.zip"},
	}

	DownloadMany(items)
	if !exist(path1) {
		t.Fatal(fmt.Sprintf("Downloaded file %s not found", path1))
	}

	if !exist(path2) {
		t.Fatal(fmt.Sprintf("Downloaded file %s not found", path2))
	}

	remove(t, path1)
	remove(t, path2)
}