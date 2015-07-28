# godownload [![Build Status](https://travis-ci.org/saromanov/godownload.svg?branch=master)](https://travis-ci.org/saromanov/godownload)

Downloading files

## Install

``` 
go get https://github.com/saromanov/godownload
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
godownload.Download("http://arxiv.org/pdf/1206.5538v3.pdf", &godownload.Options{Outpath: "fun.pdf"})
```
