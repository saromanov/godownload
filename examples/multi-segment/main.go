// Multi-segment example: fast parallel download with progress,
// throttling and checksum verification.
//
// Run: go run examples/multi-segment/main.go <url> <output>
package main

import (
	"fmt"
	"os"

	godownload "github.com/saromanov/godownload"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("usage: go run examples/multi-segment/main.go <url> <output>")
		os.Exit(2)
	}
	url, out := os.Args[1], os.Args[2]

	gd := godownload.GoDownload{Timeout: 0}
	err := gd.Download(url, &godownload.Options{
		Outpath:   out,
		Segments:  8,
		RateLimit: 20 * 1024 * 1024, // 20 MiB/s
		OnProgress: func(transferred, total int64) {
			if total > 0 {
				fmt.Printf("\r%d / %d bytes", transferred, total)
			} else {
				fmt.Printf("\r%d bytes", transferred)
			}
		},
	})
	fmt.Println()
	if err != nil {
		fmt.Fprintln(os.Stderr, "download failed:", err)
		os.Exit(1)
	}
	fmt.Println("done:", out)
}
