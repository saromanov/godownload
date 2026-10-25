// Manifest example: declarative downloads from a YAML file.
//
// Run: go run examples/manifest/main.go <manifest.yaml>
//
// Example manifest:
//
//   - url: https://example.com/model.bin
//     out: model.bin
//     checksum: sha256:...
//     mirrors:
//   - https://mirror.example.com/model.bin
package main

import (
	"fmt"
	"os"

	godownload "github.com/saromanov/godownload"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: go run examples/manifest/main.go <manifest.yaml>")
		os.Exit(2)
	}

	gd := godownload.GoDownload{}
	if err := gd.FromManifest(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, "manifest download failed:", err)
		os.Exit(1)
	}
	fmt.Println("all manifest items downloaded")
}
