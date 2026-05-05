package filer

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/disintegration/imaging"
)

type Imager struct {
	Filer
	Width   int
	Height  int
	Quality int
	rgba    *image.NRGBA
	image   image.Image

	rawOnce    sync.Once
	rawBuf     []byte
	rawLoadErr error
}

// newImager 创建 Imager 实例
func newImager(filer *Filer) (*Imager, error) {
	var err error
	imager := &Imager{
		Filer:   *filer,
		Quality: 100,
	}

	rc := filer.readCloser
	var seeker io.Seeker
	if s, ok := rc.(io.Seeker); ok {
		seeker = s
	} else {
		var data []byte
		data, err = io.ReadAll(rc)
		if err != nil {
			return imager, err
		}
		imager.readCloser = &ReadSeekCloser{bytes.NewReader(data)}
		rc = imager.readCloser
		seeker = rc.(io.Seeker)
	}

	if _, err = seeker.Seek(0, io.SeekStart); err != nil {
		return imager, err
	}

	img, _, err := image.Decode(rc)
	if err != nil {
		return imager, err
	}
	b := img.Bounds()
	imager.Width = b.Dx()
	imager.Height = b.Dy()
	imager.image = img

	return imager, nil
}

// Mode 返回图像模式
func (img *Imager) Mode() string {
	switch m := img.image.(type) {
	case *image.RGBA:
		return "RGBA"
	case *image.NRGBA:
		return "NRGBA"
	case *image.Gray:
		return "Gray"
	case *image.CMYK:
		return "CMYK"
	case *image.YCbCr:
		return "YCbCr"
	case *image.Paletted:
		return "Paletted"
	case image.Image:
		return fmt.Sprintf("%T", m)
	default:
		return "Unknown"
	}
}

// Resize 缩放图像
func (img *Imager) Resize(width, height int) error {
	if err := img.seekStart(); err != nil {
		return err
	}
	origin, _, err := image.Decode(img.readCloser)
	if err != nil {
		return err
	}
	img.rgba = imaging.Resize(origin, width, height, imaging.Lanczos)
	return nil
}

// Crop 裁剪图像
func (img *Imager) Crop(width, height int) error {
	if err := img.seekStart(); err != nil {
		return err
	}
	origin, _, err := image.Decode(img.readCloser)
	if err != nil {
		return err
	}
	img.rgba = imaging.CropAnchor(origin, width, height, imaging.Center)
	return nil
}

// Body 在已执行 Resize/Crop 时按扩展名与 Quality 编码；否则惰性读出源字节副本。
// 与嵌入的 (*Filer).Body 同名：对 *Imager 调用 Body 为本方法；读原始整流请用 img.Filer.Body()。
func (img *Imager) Body() ([]byte, error) {
	if img.rgba != nil {
		var buf bytes.Buffer
		if err := img.encodeTo(&buf); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}
	if err := img.loadSourceBytes(); err != nil {
		return nil, err
	}
	return append([]byte(nil), img.rawBuf...), nil
}

// loadSourceBytes 首次需要原始字节时读入并缓存，同时用内存流替换 readCloser，便于后续解码。
func (img *Imager) loadSourceBytes() error {
	img.rawOnce.Do(func() {
		if img.readCloser == nil {
			img.rawLoadErr = errors.New("imager: no read source")
			return
		}
		if err := img.seekStart(); err != nil {
			img.rawLoadErr = err
			return
		}
		b, err := io.ReadAll(img.readCloser)
		if err != nil {
			img.rawLoadErr = err
			return
		}
		img.rawBuf = b
		img.readCloser = &ReadSeekCloser{bytes.NewReader(b)}
	})
	return img.rawLoadErr
}

// encodeTo 按扩展名将 rgba 编码到 w（与 SaveTo 写入格式一致）。
func (img *Imager) encodeTo(w io.Writer) error {
	switch img.Ext() {
	case ".png":
		return png.Encode(w, img.rgba)
	case ".gif":
		return gif.Encode(w, img.rgba, nil)
	case ".jpg", ".jpeg":
		return jpeg.Encode(w, img.rgba, &jpeg.Options{Quality: img.Quality})
	default:
		return fmt.Errorf("invalid '%s' extension name", img.Ext())
	}
}

// SaveTo 将图像写入 path（rgba 为空则写出惰性缓存的原始字节）。
// 与嵌入的 (*Filer).SaveTo 同名：对 *Imager 调用 SaveTo 为本方法；需 Filer 的目录规则与返回值请用 img.Filer.SaveTo(...)。
func (img *Imager) SaveTo(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("imager: path is empty")
	}
	if img.rgba == nil {
		if err := img.loadSourceBytes(); err != nil {
			return err
		}
		return os.WriteFile(path, img.rawBuf, 0644)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func(f *os.File) {
		err = f.Close()
	}(f)

	return img.encodeTo(f)
}
