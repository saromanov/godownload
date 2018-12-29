# godownload 
[![Codacy Badge](https://api.codacy.com/project/badge/Grade/dcf78b0a09b4460ab21a338f9165d0d8)](https://app.codacy.com/app/saromanov/godownload?utm_source=github.com&utm_medium=referral&utm_content=saromanov/godownload&utm_campaign=Badge_Grade_Dashboard)
[![Build Status](https://travis-ci.org/saromanov/godownload.svg?branch=master)](https://travis-ci.org/saromanov/godownload)
[![Go Report Card](https://goreportcard.com/badge/github.com/saromanov/godownload)](https://goreportcard.com/report/github.com/saromanov/godownload)

Downloading files

### Install

``` 
go get https://github.com/saromanov/godownload
```

### Usage

Download file

```go
package main
import "github.com/saromanov/godownload"

func main() { 
    godownload.Download("http://arxiv.org/pdf/1206.5538v3.pdf", nil)
}

```

Download with set output file
```go
godownload.Download("http://arxiv.org/pdf/1206.5538v3.pdf", &godownload.Options{Outpath: "fun.pdf"})
```

If you have a links on the file, you can download data by this links

```go
download.FromFile("file")
```

### API
godownload.Options

Url - Url parameter needs only for DownloadMany. In the case with Download. This paremeter will be ignore

Outpath - Outpath sets the path of the downloaded file

