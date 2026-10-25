// Rate limit example: throttle the download speed.
//
// Run: go run examples/rate-limit/main.go <url> <output>
package main

import (
	"fmt"
	"os"

	godownload "github.com/saromanov/godownload"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("usage: go run examples/rate-limit/main.go <url> <output>")
		os.Exit(2)
	}
	url, out := os.Args[1], os.Args[2]

	gd := godownload.GoDownload{}
	err := gd.Download(url, &godownload.Options{
		Outpath:   out,
		RateLimit: 1024 * 1024, // 1 MiB/s
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "download failed:", err)
		os.Exit(1)
	}
	fmt.Println("done:", out)
}
