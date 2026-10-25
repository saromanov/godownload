# godownload

Библиотека для скачивания файлов по HTTP: стриминг на диск, ретраи с экспоненциальным backoff, параллельная загрузка, базовая аутентификация, упаковка в zip.

## Install

```
go get github.com/saromanov/godownload
```

## Usage

Скачивание файла (имя берётся из URL):

```go
package main

import godownload "github.com/saromanov/godownload"

func main() {
    gd := godownload.GoDownload{}
    if err := gd.Download("http://arxiv.org/pdf/1206.5538v3.pdf", nil); err != nil {
        panic(err)
    }
}
```

С указанием выходного файла:

```go
gd := godownload.GoDownload{}
err := gd.Download("http://arxiv.org/pdf/1206.5538v3.pdf", &godownload.Options{Outpath: "fun.pdf"})
```

Скачивание нескольких файлов параллельно:

```go
gd := godownload.GoDownload{}
err := gd.DownloadMany([]*godownload.Options{
    {URL: "https://example.com/a.pdf", Outpath: "a.pdf"},
    {URL: "https://example.com/b.pdf", Outpath: "b.pdf"},
})
```

Скачивание по ссылкам из файла:

```go
gd := godownload.GoDownload{}
err := gd.FromFile("links.txt")
```

Скачивание HTML-страницы:

```go
gd := godownload.GoDownload{}
err := gd.Download("https://example.com", &godownload.Options{Page: true, Outpath: "index.html"})
```

Пользовательский таймаут и ретраи:

```go
gd := godownload.GoDownload{Timeout: 30 * time.Second}
err := gd.Download("https://example.com/file.zip", &godownload.Options{Retry: 3})
```

## API

### GoDownload

Глобальные настройки (все необязательные):

| Поле | Описание |
|------|----------|
| `Overwrite` | Перезаписывать файл с тем же именем |
| `Alwaysnew` | Всегда создавать новый файл (`file_1`, `file_2`, ...) |
| `UserAgent` | User-Agent для HTTP-запросов |
| `Retry` | Количество повторных попыток при ошибках сети и 5xx |
| `Auth` | Basic-аутентификация в формате `username:password` |
| `Archive` | Формат архива для скачанного файла (`zip`) |
| `Configpath` | Путь к YAML-конфигу |
| `Outdir` | Каталог для скачанных файлов |
| `Timeout` | Таймаут HTTP-запроса (по умолчанию — без таймаута) |
| `Logger` | Свой logger (`nil` — стандартный `log`) |
| `Quiet` | Отключить весь вывод |
| `HTTPClient` | Свой `http.Client` |

### Options

Настройки одного скачивания (те же поля, где применимо) плюс:

| Поле | Описание |
|------|----------|
| `URL` | URL — нужен только для `DownloadMany` |
| `Page` | Скачать HTML-страницу вместо файла |
| `Outpath` | Путь выходного файла |

Все методы возвращают `error`. Специфические ошибки: `ErrFileExists`, `ErrUnsupportedArchive`.

> **v2.0:** методы больше не завершают процесс через `log.Fatal`/`panic` и возвращают `error`. Это несовместимое изменение по сравнению с v1.
