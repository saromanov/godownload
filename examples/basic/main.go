// Basic example: download a file with the default settings.
//
// Run: go run examples/basic/main.go <url>
package main

import (
	"fmt"
	"os"

	godownload "github.com/saromanov/godownload"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: go run examples/basic/main.go <url>")
		os.Exit(2)
	}

	gd := godownload.GoDownload{}
	if err := gd.Download(os.Args[1], nil); err != nil {
		fmt.Fprintln(os.Stderr, "download failed:", err)
		os.Exit(1)
	}
}
