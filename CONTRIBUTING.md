# Contributing to godownload

Thank you for considering a contribution.

## Development setup

Go 1.24 or newer is required.

```sh
git clone https://github.com/saromanov/godownload
cd godownload
go test -race ./...
```

## Before opening a PR

- Run `go test -race ./...` — all tests must pass.
- Run `golangci-lint run` (config in `.golangci.yml`).
- Format the code with `gofmt`/`goimports`.
- Add or update tests for any behaviour change.
- Update the README if you change the public API.
- Write commit messages and PR descriptions in English.

## Design guidelines

- The library must never call `log.Fatal`, `os.Exit` or `panic` on user-facing paths: return errors instead.
- Public API additions require doc comments starting with the identifier name.
- New options should be added to both `GoDownload` (global) and `Options` (per-download) when it makes sense, wired through the defaults in `DownloadContext`.
- Anything that can be cancelled must respect `context.Context`.

## Reporting issues

Use the issue templates (bug report / feature request). For security issues, see [SECURITY.md](SECURITY.md).
