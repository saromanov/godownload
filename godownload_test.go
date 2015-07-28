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
		t.Fatal(fmt.Sprintf("Downloaded file %s", path))
	}
	remove(t, path)
}