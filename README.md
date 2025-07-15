Filer
=====

Filer is a simple file operate library.

Support os file handle, local file path, http(s) url, base64, multipart form and bytes.

like get name, size, extension and save file, etc.

you don't care about dir and path, it will return a valid value.

## Install

```shell
go get -u github.com/hiscaler/filer-go
```

## Usage

```go
fer, err := filer.NewFiler()
if err != nil {
    log.Panic("init filer failed", err)
}
defer func() {
    _ = f.Close()
}()
err := f.Open("http://examples-1251000004.cos.ap-shanghai.myqcloud.com/sample.jpeg")
if err != nil {
    log.Panic("get http file failed", err)
}
fer.SaveTo('./a.jpeg')
```