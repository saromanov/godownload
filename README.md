# godownload

Downloading files

## Install

``` go get https://github.com/saromanov/godownload
```

## Usage

Download file

```go
package main
import
(
	"github.com/saromanov/godownload"
)

func main() { 
    godownload.Download("http://arxiv.org/pdf/1206.5538v3.pdf", nil)
}

```

Download with set output file
```go
godownload.Download("http://arxiv.org/pdf/1206.5538v3.pdf", &godownload.Item{Outpath: "fun.pdf"})
```
