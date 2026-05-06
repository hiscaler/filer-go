# filer-go

统一处理多种来源的文件流：本地路径、HTTP(S)、Base64 Data URL、`*os.File`、multipart 表单、`[]byte`
与纯文本。可查询文件名、扩展名、大小，保存到磁盘，并对常见位图做解码与缩放/裁剪。

**语言要求：** Go 1.23+

**CGO：** 不需要。WebP 编码使用纯 Go 库 [gowebp](https://github.com/KarpelesLab/gowebp)，可在 `CGO_ENABLED=0` 下构建。

---

## 安装

```bash
go get github.com/hiscaler/filer-go
```

---

## 支持的数据源（`Open`）

| 类型                                                 | 说明                                                                                                  |
|----------------------------------------------------|-----------------------------------------------------------------------------------------------------|
| `string`                                           | 见下 **「`string`：URL / 本地路径 / 纯文本」** |
| `[]byte`                                           | 原始文件字节（无文件名，部分场景下 `Name()` 可能为空）                                                                    |
| `*os.File`                                         | 已打开的文件句柄                                                                                            |
| `multipart.File`                                   | 表单文件体                                                                                               |
| `*multipart.FileHeader` / `FormFile` / `*FormFile` | 带文件名的上传字段                                                                                           |

网络请求使用包内 `http.Client`，**超时 60 秒**；非 2xx 会关闭响应体并返回错误。

### `string`：URL / 本地路径 / 纯文本

在 **非** HTTP(S)、**非** Data URL 时，先按启发式判断是否要 **尝试** 本地打开（`stringLooksLikeFilePath`），再 **`os.Open`**：

- **启发式「像路径」**：`filepath.IsAbs`；含 **`/` 或 `\`**；前缀 `./`、`.\`、`../`、`..\`；**`filepath.Ext` 合理**（扩展名段不全是数字，避免 `"version 1.2"` 误判）；以 `.` 开头的隐藏名且扩展名合理（**排除** `..` 与 **`.2`** 等纯数字伪扩展名）。
- **打开结果**：若 **`os.Open` 成功** → 按本地文件；若 **仅 `os.ErrNotExist`**（路径不存在）→ 视为 **纯文本**（因 `/tmp/a.jpg` 等形式无法与正文严格区分）；其它错误（无权限等）仍 **返回错误**。
- **一眼不像路径**：不满足启发式 → 直接 **纯文本**。

无扩展名的裸文件名（如 `README`）若未带 `./` 等前缀，会按 **纯文本** 处理；需要当文件打开时请写 **`./README`** 或使用 `[]byte` / `*os.File`。若必须区分「字符串一定是路径且不存在时要报错」，请自行 **`os.Open`** 后传入 **`*os.File`**。

---

## 快速开始

```go
package main

import (
	"log"

	"github.com/hiscaler/filer-go"
)

func main() {
	f := filer.NewFiler()
	defer func() { _ = f.Close() }()

	if err := f.Open("https://example.com/sample.jpeg"); err != nil {
		log.Fatal(err)
	}
	saved, err := f.SaveTo("./out/")
	if err != nil {
		log.Fatal(err)
	}
	_ = saved // 实际写入路径，目录以 / 或 \ 结尾时会自动拼上原文件名
}
```

---

## `Filer` 常用 API

- **`Open(any) error`**：打开数据源；每次调用会重置内部状态（含已关联的 `Imager` 缓存字段）。
- **`Name() string`**：文件名（含扩展名），与来源一致（扩展名大小写可能保留）。
- **`Title() string`**：无扩展名的文件名；与 **`Ext()`** 配合时按**不区分大小写**去掉后缀（例如 `photo.JPG` + `Ext()`
  `.jpg` → `photo`）。
- **`Ext() string`**：扩展名，**始终为小写**（如 `.jpg`）。本地文件会结合路径上的 `possibleExt` 与内容嗅探（
  `http.DetectContentType` 等）推断。
- **`Size() (int64, error)`**：长度。网络/Base64/文本使用缓存的 `size`；文件类需底层 `ReadCloser` 实现 **`io.Seeker`**。
- **`Body() ([]byte, error)`**：从头读取**完整原始流**。
- **`SaveTo(filename string) (string, error)`**：写入磁盘；**自动 `MkdirAll`**；返回最终路径。见下文「`SaveTo` 与路径」。
- **`Uri() string`**：在 **`SaveTo` 成功后**，对**相对路径**会生成以 `/` 开头的规范化 URI 片段（用于测试或展示）；绝对路径时为空字符串。
- **`Close() error`**：关闭底层流。
- **`IsEmpty() bool`**：是否零长度（依赖 `Size()`）。
- **`IsImage() bool`**：能否被 `image.DecodeConfig` 识别为图片；嗅探最多读取约 **64KiB**（便于 TIFF 等格式）。
- **`Imager() (*Imager, error)`**：在 `IsImage()` 为真时解码为 `Imager`；否则返回 `filer: not an image`。

---

## `SaveTo` 与路径注意事项

1. **目录保存**：`filename` 以 `/` 或 `\` 结尾（可先 `TrimSpace`）时，视为目录，会在末尾追加 **`Name()`**；若 `Name()` 为空则用
   **`纳秒时间戳 + Ext()`** 作为文件名。
2. **跨平台**：会先 **`ReplaceAll('\', '/')` 再 `filepath.FromSlash`**，避免在 Unix 上出现 `.\tmp` 这类仅 Windows 可用的写法。
3. **相对路径与 `Uri()`**：保存后相对路径会写入 `Uri()`（前导 `./` 会去掉，并保证以 `/` 开头）。

---

## `Imager`（图像处理）

通过 **`f.Imager()`** 获取。每次调用都会**重新解码**（请缓存返回的 `*Imager` 复用，避免重复开销）。内部嵌入 **`Filer`**，解码依赖
`image.Decode`；包内 **`imageformats.go`** 空白导入以注册 **BMP、TIFF、WebP** 解码（WebP 为纯 Go）。

| 方法                              | 说明                                                                                           |
|---------------------------------|----------------------------------------------------------------------------------------------|
| **`Resize(w, h int) error`**    | 按宽高缩放（Lanczos）。                                                                              |
| **`Crop(w, h int) error`**      | 自中心裁剪。                                                                                       |
| **`Width()` / `Height()`**      | 只读：解码后的像素尺寸；**Resize**/**Crop** 成功后会更新为当前位图大小。                                                   |
| **`Quality()` / `SetQuality(q)`** | 有损输出质量 **1–100**，默认 **100**；通过 **`SetQuality`** 修改（可链式），**`Quality()`** 读取当前值。                 |
| **`Body() ([]byte, error)`**    | 若已 **`Resize`/`Crop`**（存在 `rgba`）：按当前 **`Ext()`** 与 **`SetQuality`** 编码后返回；否则惰性读取并缓存**原始字节**副本。 |
| **`SaveTo(path string) error`** | 有 `rgba` 时按扩展名编码写入；否则写出缓存的原始字节。路径需含**完整文件名**（与 `Filer.SaveTo` 的目录规则不同）。                      |

**编码与扩展名（`encodeTo`）**：扩展名比较**不区分大小写**。支持 *
*`.png`、`.gif`、`.jpg`/`.jpeg`、`.bmp`、`.tif`/`.tiff`、`.webp`**。其它扩展名返回 **`imager: invalid '...' extension name`**。

**WebP**：使用有损编码；质量为 0 时库内按 **75** 处理，大于 100 按 **100** 截断。

非可 Seek 的流在首次需要时会**整段读入内存**再解码，大文件请注意内存占用。

---

## `Filer` 与 `Imager` 同名方法（嵌入遮蔽）

`Imager` 嵌入了 `Filer`。对 **`*Imager`** 直接调用 **`Body` / `SaveTo`** 时，使用的是 **`Imager` 的实现**，而不是 `Filer`
的。

| 方法                | `*Filer`                          | `*Imager`                      |
|-------------------|-----------------------------------|--------------------------------|
| **`Body()`**      | 原始完整流                             | 已处理则按格式编码；否则原始字节的惰性副本          |
| **`SaveTo(...)`** | **`(string, error)`**，支持目录后缀、返回路径 | **`error`**，需完整文件路径，按图像或原始字节写入 |

需要 **`Filer` 行为**时请显式写：**`img.Filer.Body()`**、**`img.Filer.SaveTo(filename)`**。

---

## 依赖摘要

- 图像处理：[disintegration/imaging](https://github.com/disintegration/imaging)
- 额外解码/编码：`golang.org/x/image`（BMP、TIFF）、[KarpelesLab/gowebp](https://github.com/KarpelesLab/gowebp)（WebP，纯 Go）
- 其它：`gopkg.in/guregu/null.v4`（`FileInfo` 等结构体字段类型）

---

## 并发与 goroutine 安全

**`*Filer` 与 `*Imager` 均不是线程安全的**（与多数带 `io.ReadCloser`、可变内部状态的类型相同）。库内仅有 `Imager` 的
`loadSourceBytes` 使用 `sync.Once`，仅保证「首次把流读入内存」单次执行，**不保证**多 goroutine 同时调用 `Resize`、`Body`、
`SaveTo`、`Filer.Body` 等是安全的。

### 推荐做法

1. **一实例一 goroutine（最省事）**  
   每个 HTTP 请求、每个 worker 任务使用**各自的** `filer.NewFiler()`，在同一线程内顺序调用 `Open` → `Body` / `SaveTo` /
   `Imager()` → `Close`。需要并行处理多份文件时，在多个 goroutine 里各建一个 `Filer`，**不要**把同一个 `*Filer` 传给多个
   goroutine。

2. **必须共享同一个 `*Filer` 时**  
   在业务层用 `sync.Mutex`（或 `RWMutex`，若你能严格区分只读阶段）包住对该实例的**全部**操作（含 `Open`、`Body`、`SaveTo`、
   `IsImage`、`Imager`、`Close` 等），避免与内部的 `seekStart`、共享流读写交错。

3. **`*Imager`**  
   同样**不要**多 goroutine 并发调用同一实例的 `Resize`、`Crop`、`Body`、`SaveTo`；若要对同一图源并发做多种导出，应先**各自**
   `Open` 一份 `[]byte` 副本并分别建 `Filer`/`Imager`，或为 `Imager` 外包一层互斥。

4. **不必把库改成默认加锁**  
   常见服务模型下「每请求一个新 `Filer`」即可；若将来需要对外承诺线程安全，更适合在应用侧做包装类型，而不是假设所有调用都会并发。

---

## 使用注意（汇总）

1. **务必处理 `Open` / `SaveTo` / `Imager` 的错误**；HTTP 失败或非图片调用 `Imager()` 都会返回明确错误。
2. **`Ext()` 一律小写**；依赖扩展名做分支时请统一用小写或自行 `ToLower`。
3. **`Open([]byte)`** 无路径时 **`Name()`** 可能为空，仅用 **`SaveTo("./dir/")`** 时会生成时间戳文件名。
4. **`Size()`** 对非网络类源要求 **`io.Seeker`**；纯 `io.ReadCloser` 会报错。
5. **图片判断**依赖注册格式与文件头；罕见格式或损坏文件可能 **`IsImage()` 为 false**。
6. **GIF 编码**使用 `gif.Encode(..., nil)`，多帧动图只会按当前位图状态编码，**不保留动画元数据**。
7. **并发**：见上文「**并发与 goroutine 安全**」。
8. 使用完毕后调用 **`Close()`** 释放网络连接或文件句柄。

---
