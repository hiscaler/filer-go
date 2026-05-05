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
f := filer.NewFiler()
defer func() { _ = f.Close() }()

err := f.Open("http://examples-1251000004.cos.ap-shanghai.myqcloud.com/sample.jpeg")
if err != nil {
    log.Panic("get http file failed", err)
}
_, err = f.SaveTo("./a.jpeg")
```

## Imager and embedded `Filer`

`Imager` embeds `Filer`. Both types define **`Body`** and **`SaveTo`**, but with different signatures and behaviour:

| Method | On `*Filer` | On `*Imager` |
|--------|-------------|----------------|
| **`Body()`** | Reads the **full raw stream** from `readCloser`. | Returns **image-related bytes**: encoded output after Resize/Crop, or lazily loaded **original file bytes** if you have not processed the image yet. |
| **`SaveTo(...)`** | **`SaveTo(filename string) (string, error)`** — directory suffix handling, returns the saved path. | **`SaveTo(path string) error`** — writes the image (or raw bytes when there is no processed bitmap) to **`path`**. |

When you call **`img.Body()`** or **`img.SaveTo(path)`** on **`*Imager`**, Go selects the **`Imager` implementation**; the promoted **`Filer` methods with the same names are not used** for that call.

To use the **`Filer` behaviour on an `Imager` value**, call through the embedded field explicitly:

- **`img.Filer.Body()`** — raw stream bytes as with a standalone `Filer`.
- **`img.Filer.SaveTo(filename)`** — full `SaveTo` with returned path.
